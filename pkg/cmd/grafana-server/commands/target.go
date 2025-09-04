package commands

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	"github.com/grafana/grafana/pkg/services/featuremgmt"
	"github.com/urfave/cli/v2"

	"github.com/grafana/grafana/pkg/api"
	"github.com/grafana/grafana/pkg/infra/log"
	"github.com/grafana/grafana/pkg/infra/metrics"
	"github.com/grafana/grafana/pkg/server"
	"github.com/grafana/grafana/pkg/services/apiserver/standalone"
	"github.com/grafana/grafana/pkg/setting"
)

// INFO: 创建target子命令,用于启动dskit定义的服务
func TargetCommand(version, commit, buildBranch, buildstamp string) *cli.Command {
	return &cli.Command{
		Name:  "target",
		Usage: "target specific grafana services",
		Flags: commonFlags,
		Action: func(context *cli.Context) error {
			return RunTargetServer(standalone.BuildInfo{
				Version:     version,
				Commit:      commit,
				BuildBranch: buildBranch,
				BuildStamp:  buildstamp,
			}, context)
		},
	}
}

func RunTargetServer(opts standalone.BuildInfo, cli *cli.Context) error {
	// INFO: 判断是否只是打印版本信息
	if Version || VerboseVersion {
		fmt.Printf("Version %s (commit: %s, branch: %s)\n", opts.Version, opts.Commit, opts.BuildBranch)
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

	if err := setupProfiling(Profile, ProfileAddr, ProfilePort, ProfileBlockRate, ProfileMutexFraction); err != nil {
		return err
	}
	if err := setupTracing(Tracing, TracingFile, logger); err != nil {
		return err
	}

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

	SetBuildInfo(opts)
	checkPrivileges()

	configOptions := strings.Split(ConfigOverrides, " ")
	// INFO: 创建配置对象
	// INFO: 包括从配置文件中加载及命令行参数覆盖
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

	// Initialize the OpenFeature client with the configuration
	if err := featuremgmt.InitOpenFeatureWithCfg(cfg); err != nil {
		return err
	}

	// INFO: 通过配置对象初始化模块服务(但是不启动)
	// INFO: InitializeModuleServer方法是wire生成的
	s, err := server.InitializeModuleServer(
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
	// INFO: 监听系统信号
	go listenToSystemSignals(ctx, s)
	// INFO: 阻塞直到所有服务都退出
	return s.Run()
}
