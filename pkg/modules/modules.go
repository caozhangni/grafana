package modules

import (
	"context"
	"errors"
	"strings"

	"github.com/grafana/dskit/modules"
	"github.com/grafana/dskit/services"

	"github.com/grafana/grafana/pkg/infra/log"
)

// INFO: 管理模块的生命周期
type Engine interface {
	Run(context.Context) error
	Shutdown(context.Context, string) error
}

// INFO: 用于管理模块(注册)
type Manager interface {
	RegisterModule(name string, fn initFn)
	RegisterInvisibleModule(name string, fn initFn)
}

var _ Engine = (*service)(nil)
var _ Manager = (*service)(nil)

// service manages the registration and lifecycle of modules.
// INFO: service 管理模块的注册和生命周期
// INFO: 实现了Engine和Manager接口
type service struct {
	log     log.Logger
	targets []string

	moduleManager  *modules.Manager // INFO: 使用dskit的模块管理器
	serviceManager *services.Manager // INFO: 使用dskit的service管理器
	serviceMap     map[string]services.Service
}

func New(
	targets []string,
) *service {
	logger := log.New("modules")

	return &service{
		log:     logger,
		targets: targets,

		moduleManager: modules.NewManager(logger),
		serviceMap:    map[string]services.Service{},
	}
}

// Run starts all registered modules.
// INFO: 运行所有已注册的模块
// NOTE: 这里会阻塞直到所有服务都退出
func (m *service) Run(ctx context.Context) error {
	var err error

	// INFO: 给模块添加依赖的模块
	for mod, targets := range dependencyMap {
		// INFO: 没有注册过的模块无需添加依赖
		if !m.moduleManager.IsModuleRegistered(mod) {
			continue
		}
		// INFO: 添加依赖
		if err := m.moduleManager.AddDependency(mod, targets...); err != nil {
			return err
		}
	}

	// INFO: 初始化模块对应的service
	m.serviceMap, err = m.moduleManager.InitModuleServices(m.targets...)
	if err != nil {
		return err
	}

	// if no modules are registered, we don't need to start the service manager
	if len(m.serviceMap) == 0 {
		return nil
	}

	// INFO: 创建Service对象的切片
	svcs := make([]services.Service, 0, len(m.serviceMap))
	for _, s := range m.serviceMap {
		svcs = append(svcs, s)
	}

	// INFO: 通过service对象的切片创建service管理器
	m.serviceManager, err = services.NewManager(svcs...)
	if err != nil {
		return err
	}

	// we don't need to continue if no modules are registered.
	// this behavior may need to change if dskit services replace the
	// current background service registry.
	if len(m.serviceMap) == 0 {
		m.log.Warn("No modules registered...")
		<-ctx.Done()
		return nil
	}

	// INFO: 创建serviceManager的监听器
	// INFO: 该监听器会定义一些回调函数,用于在serviceManager状态发生变化时执行
	listener := newServiceListener(m.log, m)
	// INFO: 注册listener到serviceManger中
	m.serviceManager.AddListener(listener)

	m.log.Debug("Starting module service manager", "targets", strings.Join(m.targets, ","))
	// wait until a service fails or stop signal was received
	// INFO: 异步启动serviceManager管理的所有service
	// NOTE: 这里并不会阻塞,会立即返回(官方的注释有点误导)
	err = m.serviceManager.StartAsync(ctx)
	if err != nil {
		return err
	}

	stopCtx := context.Background()
	// IMPT: 这里会阻塞等待stopCtx结束或者serviceManager的stoppedCh被关闭
	// IMPT: 这里的stopCtx实际上并不会有任何的作用,因为它只是一个context.Background,不是带取消功能的
	if err = m.serviceManager.AwaitStopped(stopCtx); err != nil {
		m.log.Error("Failed to stop module service manager", "error", err)
		return err
	}

	failed := m.serviceManager.ServicesByState()[services.Failed]
	for _, f := range failed {
		// the service listener will log error details for all modules that failed,
		// so here we return the first error that is not ErrStopProcess
		if !errors.Is(f.FailureCase(), modules.ErrStopProcess) {
			return f.FailureCase()
		}
	}

	return nil
}

// Shutdown stops all modules and waits for them to stop.
func (m *service) Shutdown(ctx context.Context, reason string) error {
	if m.serviceManager == nil {
		m.log.Debug("No modules registered, nothing to stop...")
		return nil
	}
	m.serviceManager.StopAsync()
	m.log.Info("Awaiting services to be stopped...", "reason", reason)
	return m.serviceManager.AwaitStopped(ctx)
}

type initFn func() (services.Service, error)

// RegisterModule registers a module with the dskit module manager.
// INFO: 注册一个模块，并将其注册到 dskit 模块管理器中
func (m *service) RegisterModule(name string, fn initFn) {
	// INFO: 委派给dskit的模块管理器
	m.moduleManager.RegisterModule(name, fn)
}

// RegisterInvisibleModule registers an invisible module with the dskit module manager.
// Invisible modules are not visible to the user, and are intended to be used as dependencies.
// INFO: 注册一个不可见的模块，并将其注册到 dskit 模块管理器中
func (m *service) RegisterInvisibleModule(name string, fn initFn) {
	// INFO: 委派给dskit的模块管理器
	m.moduleManager.RegisterModule(name, fn, modules.UserInvisibleModule)
}

// IsModuleEnabled returns true if the module is enabled.
func (m *service) IsModuleEnabled(name string) bool {
	return stringsContain(m.targets, name)
}
