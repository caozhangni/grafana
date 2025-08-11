package main

import (
	"fmt"
	"os"

	"github.com/fatih/color"
	"github.com/urfave/cli/v2"

	gcli "github.com/grafana/grafana/pkg/cmd/grafana-cli/commands"
	"github.com/grafana/grafana/pkg/cmd/grafana-server/commands"
	"github.com/grafana/grafana/pkg/server"
	"github.com/grafana/grafana/pkg/services/apiserver/standalone"
)

// The following variables cannot be constants, since they can be overridden through the -X link flag
// INFO: 如下的变量不能是常量，因为编译的时候它们可以通过-X标志被重写
// var version = "9.2.0"
// NOTE: 这里我直接修改了版本号(最近的版本号需要从package.json文件中的version字段获取)
// NOTE: 因为在debug的时候，grafana没有提供一种方式可以覆盖这个变量
// NOTE: 一些逻辑会检查版本号，比如插件的版本兼容逻辑，所以也是无奈之举
var version = "12.2.0-pre"
var commit = gcli.DefaultCommitValue
var enterpriseCommit = gcli.DefaultCommitValue
var buildBranch = "main"
var buildstamp string

func main() {
	app := MainApp()

	if err := app.Run(os.Args); err != nil {
		fmt.Printf("%s: %s %s\n", color.RedString("Error"), color.RedString("✗"), err)
		os.Exit(1)
	}

	os.Exit(0)
}

// INFO: 创建grafana命令行应用
func MainApp() *cli.App {
	app := &cli.App{
		Name:  "grafana",
		Usage: "Grafana server and command line interface",
		Authors: []*cli.Author{
			{
				Name:  "Grafana Project",
				Email: "hello@grafana.com",
			},
		},
		Version: version,
		// INFO: 支持两个子命令: cli和server
		Commands: []*cli.Command{
			gcli.CLICommand(version),
			commands.ServerCommand(version, commit, enterpriseCommit, buildBranch, buildstamp),
		},
		CommandNotFound:      cmdNotFound,
		EnableBashCompletion: true,
	}

	// Set the global build info
	buildInfo := standalone.BuildInfo{
		Version:          version,
		Commit:           commit,
		EnterpriseCommit: enterpriseCommit,
		BuildBranch:      buildBranch,
		BuildStamp:       buildstamp,
	}
	commands.SetBuildInfo(buildInfo)

	// Add the enterprise command line to build an api server
	f, err := server.InitializeAPIServerFactory()
	if err == nil {
		cmd := f.GetCLICommand(buildInfo)
		if cmd != nil {
			app.Commands = append(app.Commands, cmd)
		}
	}

	return app
}

func cmdNotFound(c *cli.Context, command string) {
	fmt.Printf(
		"%s: '%s' is not a %s command. See '%s --help'.\n",
		c.App.Name,
		command,
		c.App.Name,
		os.Args[0],
	)
	os.Exit(1)
}
