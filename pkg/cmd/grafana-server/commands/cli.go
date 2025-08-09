package commands

import (
	"context"
	"fmt"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/grafana/grafana/pkg/services/featuremgmt"
	_ "github.com/grafana/pyroscope-go/godeltaprof/http/pprof"

	"github.com/urfave/cli/v2"

	"github.com/grafana/grafana/pkg/api"
	gcli "github.com/grafana/grafana/pkg/cmd/grafana-cli/commands"
	"github.com/grafana/grafana/pkg/infra/log"
	"github.com/grafana/grafana/pkg/infra/metrics"
	"github.com/grafana/grafana/pkg/infra/process"
	"github.com/grafana/grafana/pkg/server"
	"github.com/grafana/grafana/pkg/services/apiserver/standalone"
	"github.com/grafana/grafana/pkg/setting"
)

// INFO: 创建server命令并返回
func ServerCommand(version, commit, enterpriseCommit, buildBranch, buildstamp string) *cli.Command {
	return &cli.Command{
		// INFO: 命令名称
		Name:  "server",
		Usage: "run the grafana server",
		// INFO: 定义命令行参数
		Flags: commonFlags,
		// INFO: 执行命令的时候被调用的函数
		Action: func(context *cli.Context) error {
			return RunServer(standalone.BuildInfo{
				Version:          version,
				Commit:           commit,
				EnterpriseCommit: enterpriseCommit,
				BuildBranch:      buildBranch,
				BuildStamp:       buildstamp,
			}, context)
		},
		// INFO: 这里又嵌套了一个命令
		Subcommands: []*cli.Command{TargetCommand(version, commit, buildBranch, buildstamp)},
	}
}

// INFO: 运行server
func RunServer(opts standalone.BuildInfo, cli *cli.Context) error {
	// INFO: 这是两个打印版本的flag
	// 有就打印版本信息
	if Version || VerboseVersion {
		if opts.EnterpriseCommit != gcli.DefaultCommitValue && opts.EnterpriseCommit != "" {
			fmt.Printf("Version %s (commit: %s, branch: %s, enterprise-commit: %s)\n", opts.Version, opts.Commit, opts.BuildBranch, opts.EnterpriseCommit)
		} else {
			fmt.Printf("Version %s (commit: %s, branch: %s)\n", opts.Version, opts.Commit, opts.BuildBranch)
		}
		if VerboseVersion {
			fmt.Println("Dependencies:")
			if info, ok := debug.ReadBuildInfo(); ok {
				for _, dep := range info.Deps {
					fmt.Println(dep.Path, dep.Version)
				}
			}
		}
		return nil
	}

	logger := log.New("cli")
	defer func() {
		if err := log.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to close log: %s\n", err)
		}
	}()

	// INFO: 设置Go语言的性能分析（profiling）功能
	if err := setupProfiling(Profile, ProfileAddr, ProfilePort, ProfileBlockRate, ProfileMutexFraction); err != nil {
		return err
	}
	// INFO: 设置Go语言的执行跟踪功能(和可观测性中的tracing不是同一个概念)
	if err := setupTracing(Tracing, TracingFile, logger); err != nil {
		return err
	}

	// INFO: 处理整个server的panic(兜底)
	// INFO: 确保在任何情况下，server退出之前，日志都会被写入到日志文件中
	defer func() {
		// If we've managed to initialize them, this is the last place
		// where we're able to log anything that'll end up in Grafana's
		// log files.
		// Since operators are not always looking at stderr, we'll try
		// to log any and all panics that are about to crash Grafana to
		// our regular log locations before exiting.
		if r := recover(); r != nil {
			reason := fmt.Sprintf("%v", r)
			logger.Error("Critical error", "reason", reason, "stackTrace", string(debug.Stack()))
			panic(r)
		}
	}()

	// INFO: 设置创建信息
	SetBuildInfo(opts)
	// INFO: 检查grafana是否以root用户运行,有的话打印警告
	checkPrivileges()

	configOptions := strings.Split(ConfigOverrides, " ")
	// INFO: 从命令行参数中创建配置对象
	cfg, err := setting.NewCfgFromArgs(setting.CommandLineArgs{
		Config:   ConfigFile,
		HomePath: HomePath,
		// tailing arguments have precedence over the options string
		Args: append(configOptions, cli.Args().Slice()...),
	})
	if err != nil {
		return err
	}

	metrics.SetBuildInformation(metrics.ProvideRegisterer(), opts.Version, opts.Commit, opts.BuildBranch, getBuildstamp(opts))

	// Initialize the OpenFeature feature flag system
	if err := featuremgmt.InitOpenFeatureWithCfg(cfg); err != nil {
		return err
	}

	// INFO: 初始化server对象
	// INFO: Intialize方法是wire生成的
	s, err := server.Initialize(
		cfg,
		server.Options{
			PidFile:     PidFile,
			Version:     opts.Version,
			Commit:      opts.Commit,
			BuildBranch: opts.BuildBranch,
		},
		api.ServerOptions{},
	)
	if err != nil {
		return err
	}

	ctx := context.Background()
	// INFO: 启动协程监听系统信号
	go listenToSystemSignals(ctx, s)
	// INFO: 启动服务
	return s.Run()
}

func validPackaging(packaging string) string {
	validTypes := []string{"dev", "deb", "rpm", "docker", "brew", "hosted", "unknown"}
	for _, vt := range validTypes {
		if packaging == vt {
			return packaging
		}
	}
	return "unknown"
}

// a small interface satisfied by the server and moduleserver
type gserver interface {
	Shutdown(context.Context, string) error
}

// INFO: 监听系统信号
func listenToSystemSignals(ctx context.Context, s gserver) {
	signalChan := make(chan os.Signal, 1)
	sighupChan := make(chan os.Signal, 1)

	// INFO: 监听reload信号
	signal.Notify(sighupChan, syscall.SIGHUP)
	// INFO: 监听中断(ctrl+c)和终止信号(kill -15)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)

	for {
		select {
		case <-sighupChan:
			if err := log.Reload(); err != nil {
				fmt.Fprintf(os.Stderr, "Failed to reload loggers: %s\n", err)
			}
		case sig := <-signalChan:
			// INFO: 设置一个30秒的Shutdown超时
			ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			// ctx, cancel := context.WithTimeout(ctx, 3000000*time.Second)
			defer cancel()
			// INFO: 这个Shutdown方法会处理超时的情况
			if err := s.Shutdown(ctx, fmt.Sprintf("System signal: %s", sig)); err != nil {
				// INFO: 这是把错误信息写入到stderr
				fmt.Fprintf(os.Stderr, "Timed out waiting for server to shut down\n")
			}
			return
		}
	}
}

func checkPrivileges() {
	elevated, err := process.IsRunningWithElevatedPrivileges()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error checking server process execution privilege. error: %s\n", err.Error())
	}
	if elevated {
		fmt.Println("Grafana server is running with elevated privileges. This is not recommended")
	}
}
