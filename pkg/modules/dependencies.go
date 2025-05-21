package modules

const (
	// All includes all modules necessary for Grafana to run as a standalone server
	All string = "all"

	Core                  string = "core"
	MemberlistKV          string = "memberlistkv"
	// NOTE: 注意这个模块实际并没有被注册,所以配置这个模块在启动的时候会报找不到的错误
	GrafanaAPIServer      string = "grafana-apiserver"
	StorageRing           string = "storage-ring"
	Distributor           string = "distributor"
	StorageServer         string = "storage-server"
	ZanzanaServer         string = "zanzana-server"
	InstrumentationServer string = "instrumentation-server"
	FrontendServer        string = "frontend-server"
)

// INFO: 定义模块之间的依赖关系
var dependencyMap = map[string][]string{
	MemberlistKV:     {InstrumentationServer},
	StorageRing:      {InstrumentationServer, MemberlistKV},
	GrafanaAPIServer: {InstrumentationServer},
	StorageServer:    {InstrumentationServer, StorageRing},
	ZanzanaServer:    {InstrumentationServer},
	Distributor:      {InstrumentationServer, MemberlistKV, StorageRing},
	Core:             {},
	All:              {Core},
	FrontendServer:   {},
}
