package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"sync"

	"github.com/grafana/grafana/pkg/services/featuremgmt"
	"golang.org/x/sync/errgroup"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/grafana/grafana/pkg/api"
	_ "github.com/grafana/grafana/pkg/extensions"
	"github.com/grafana/grafana/pkg/infra/log"
	"github.com/grafana/grafana/pkg/infra/metrics"
	"github.com/grafana/grafana/pkg/infra/usagestats/statscollector"
	"github.com/grafana/grafana/pkg/registry"
	"github.com/grafana/grafana/pkg/services/accesscontrol"
	"github.com/grafana/grafana/pkg/services/provisioning"
	"github.com/grafana/grafana/pkg/setting"
)

// Options contains parameters for the New function.
type Options struct {
	HomePath    string
	PidFile     string
	Version     string
	Commit      string
	BuildBranch string
	Listener    net.Listener
}

// New returns a new instance of Server.
// INFO: Server的构造方法,被依赖注入框架调用
func New(opts Options, cfg *setting.Cfg, httpServer *api.HTTPServer, roleRegistry accesscontrol.RoleRegistry,
	provisioningService provisioning.ProvisioningService, backgroundServiceProvider registry.BackgroundServiceRegistry,
	usageStatsProvidersRegistry registry.UsageStatsProvidersRegistry, statsCollectorService *statscollector.Service,
	promReg prometheus.Registerer,
) (*Server, error) {
	statsCollectorService.RegisterProviders(usageStatsProvidersRegistry.GetServices())
	s, err := newServer(opts, cfg, httpServer, roleRegistry, provisioningService, backgroundServiceProvider, promReg)
	if err != nil {
		return nil, err
	}

	if err := s.Init(); err != nil {
		return nil, err
	}

	return s, nil
}

func newServer(opts Options, cfg *setting.Cfg, httpServer *api.HTTPServer, roleRegistry accesscontrol.RoleRegistry,
	provisioningService provisioning.ProvisioningService, backgroundServiceProvider registry.BackgroundServiceRegistry,
	promReg prometheus.Registerer,
) (*Server, error) {
	rootCtx, shutdownFn := context.WithCancel(context.Background())
	// INFO: 基于rootCtx创建一个errgroup及childCtx
	childRoutines, childCtx := errgroup.WithContext(rootCtx)

	s := &Server{
		promReg:             promReg,
		// NOTE: 注意这里context是childCtx，而不是rootCtx
		context:             childCtx,
		childRoutines:       childRoutines,
		HTTPServer:          httpServer,
		provisioningService: provisioningService,
		roleRegistry:        roleRegistry,
		shutdownFn:          shutdownFn,
		shutdownFinished:    make(chan struct{}),
		log:                 log.New("server"),
		cfg:                 cfg,
		pidFile:             opts.PidFile,
		version:             opts.Version,
		commit:              opts.Commit,
		buildBranch:         opts.BuildBranch,
		backgroundServices:  backgroundServiceProvider.GetServices(),
	}

	return s, nil
}

// Server is responsible for managing the lifecycle of services. This is the
// core Server implementation which starts the entire Grafana server. Use
// ModuleServer to launch specific modules.
// INFO: 表示整个Grafana服务
type Server struct {
	context          context.Context // INFO: 注意这里context是childCtx，而不是rootCtx
	shutdownFn       context.CancelFunc // INFO: 用于取消context
	childRoutines    *errgroup.Group // INFO: 服务下的子协程
	log              log.Logger
	cfg              *setting.Cfg
	shutdownOnce     sync.Once
	shutdownFinished chan struct{}
	isInitialized    bool
	mtx              sync.Mutex

	pidFile            string
	version            string
	commit             string
	buildBranch        string
	backgroundServices []registry.BackgroundService

	// IMPT: 实际看下来这个单独的HTTPServer已经没什么用了(目前就只有很多的单元测试在引用)
	// IMPT: 注意目前的HTTPServer是作为一个标准化的BackgroundService进行启动的
	HTTPServer          *api.HTTPServer
	roleRegistry        accesscontrol.RoleRegistry
	provisioningService provisioning.ProvisioningService
	promReg             prometheus.Registerer
}

// Init initializes the server and its services.
func (s *Server) Init() error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	if s.isInitialized {
		return nil
	}
	s.isInitialized = true

	if err := s.writePIDFile(); err != nil {
		return err
	}

	if err := metrics.SetEnvironmentInformation(s.promReg, s.cfg.MetricsGrafanaEnvironmentInfo); err != nil {
		return err
	}

	if err := s.roleRegistry.RegisterFixedRoles(s.context); err != nil {
		return err
	}

	// Initialize the OpenFeature feature flag system
	if err := featuremgmt.InitOpenFeatureWithCfg(s.cfg); err != nil {
		return err
	}

	return s.provisioningService.RunInitProvisioners(s.context)
}

// Run initializes and starts services. This will block until all services have
// exited. To initiate shutdown, call the Shutdown method in another goroutine.
// INFO: 启动服务
// INFO: 该方法会阻塞等待所有服务启动退出
// INFO: 需要通过其他协程调用Shutdown方法来触发关闭
func (s *Server) Run() error {
	defer close(s.shutdownFinished)

	if err := s.Init(); err != nil {
		return err
	}

	services := s.backgroundServices

	// Start background services.
	for _, svc := range services {
		if registry.IsDisabled(svc) {
			continue
		}

		service := svc
		serviceName := reflect.TypeOf(service).String()
		// INFO: 单个后台服务作为一个协程启动
		s.childRoutines.Go(func() error {
			select {
			case <-s.context.Done():
				return s.context.Err()
			default:
			}
			s.log.Debug("Starting background service", "service", serviceName)
			// IMPT: 传进Run方法的context是能够处理ctx.Done()的
			// INFO: 这里会阻塞等待服务退出
			err := service.Run(s.context)
			// Do not return context.Canceled error since errgroup.Group only
			// returns the first error to the caller - thus we can miss a more
			// interesting error.
			if err != nil && !errors.Is(err, context.Canceled) {
				s.log.Error("Stopped background service", "service", serviceName, "reason", err)
				return fmt.Errorf("%s run error: %w", serviceName, err)
			}
			s.log.Debug("Stopped background service", "service", serviceName, "reason", err)
			return nil
		})
	}

	s.notifySystemd("READY=1")

	s.log.Debug("Waiting on services...")
	return s.childRoutines.Wait()
}

// Shutdown initiates Grafana graceful shutdown. This shuts down all
// running background services. Since Run blocks Shutdown supposed to
// be run from a separate goroutine.
// INFO: Shutdown启动Grafana的优雅关闭流程
// INFO: 这会关闭所有正在运行的后台服务
// INFO: 由于Run会阻塞，Shutdown应该在一个单独的goroutine中运行
func (s *Server) Shutdown(ctx context.Context, reason string) error {
	var err error
	s.shutdownOnce.Do(func() {
		s.log.Info("Shutdown started", "reason", reason)
		// Call cancel func to stop background services.
		// INFO: 关闭context,那些后台服务会处理ctx.Done()并退出
		s.shutdownFn()
		// Wait for server to shut down
		select {
		case <-s.shutdownFinished:
			s.log.Debug("Finished waiting for server to shut down")
		case <-ctx.Done():
			s.log.Warn("Timed out while waiting for server to shut down")
			err = fmt.Errorf("timeout waiting for shutdown")
		}
	})

	return err
}

// writePIDFile retrieves the current process ID and writes it to file.
func (s *Server) writePIDFile() error {
	if s.pidFile == "" {
		return nil
	}

	// Ensure the required directory structure exists.
	err := os.MkdirAll(filepath.Dir(s.pidFile), 0700)
	if err != nil {
		s.log.Error("Failed to verify pid directory", "error", err)
		return fmt.Errorf("failed to verify pid directory: %s", err)
	}

	// Retrieve the PID and write it to file.
	pid := strconv.Itoa(os.Getpid())
	if err := os.WriteFile(s.pidFile, []byte(pid), 0644); err != nil {
		s.log.Error("Failed to write pidfile", "error", err)
		return fmt.Errorf("failed to write pidfile: %s", err)
	}

	s.log.Info("Writing PID file", "path", s.pidFile, "pid", pid)
	return nil
}

// notifySystemd sends state notifications to systemd.
func (s *Server) notifySystemd(state string) {
	notifySocket := os.Getenv("NOTIFY_SOCKET")
	if notifySocket == "" {
		s.log.Debug(
			"NOTIFY_SOCKET environment variable empty or unset, can't send systemd notification")
		return
	}

	socketAddr := &net.UnixAddr{
		Name: notifySocket,
		Net:  "unixgram",
	}
	conn, err := net.DialUnix(socketAddr.Net, nil, socketAddr)
	if err != nil {
		s.log.Warn("Failed to connect to systemd", "err", err, "socket", notifySocket)
		return
	}
	defer func() {
		if err := conn.Close(); err != nil {
			s.log.Warn("Failed to close connection", "err", err)
		}
	}()

	_, err = conn.Write([]byte(state))
	if err != nil {
		s.log.Warn("Failed to write notification to systemd", "err", err)
	}
}
