package server

import (
	"github.com/grafana/dskit/dns"
	"github.com/grafana/dskit/flagext"
	"github.com/grafana/dskit/kv"
	"github.com/grafana/dskit/kv/codec"
	"github.com/grafana/dskit/kv/memberlist"
	"github.com/grafana/dskit/ring"
	"github.com/grafana/dskit/services"
	"github.com/grafana/grafana/pkg/infra/log"
	"github.com/grafana/grafana/pkg/setting"
	"github.com/prometheus/client_golang/prometheus"
)

// INFO: 初始化memberlist的kv存储
func (ms *ModuleServer) initMemberlistKV() (services.Service, error) {
	logger := log.New("memberlist")

	dnsProviderReg := prometheus.WrapRegistererWithPrefix(
		"grafana",
		prometheus.WrapRegistererWith(
			prometheus.Labels{"component": "memberlist"},
			ms.registerer,
		),
	)
	// INFO: 创建dns提供者
	dnsProvider := dns.NewProvider(logger, dnsProviderReg, dns.GolangResolverType)

	// INFO: 创建kv存储
	KVStore := kv.Config{Store: "memberlist"}

	// INFO: 创建memberlist的kv存储
	memberlistKVsvc := memberlist.NewKVInitService(toMemberlistConfig(ms.cfg), logger, dnsProvider, ms.registerer)
	KVStore.MemberlistKV = memberlistKVsvc.GetMemberlistKV

	ms.MemberlistKVConfig = KVStore

	ms.httpServerRouter.Path("/memberlist").Methods("GET", "POST").Handler(memberlistKVsvc)

	return memberlistKVsvc, nil
}

func toMemberlistConfig(cfg *setting.Cfg) *memberlist.KVConfig {
	memberlistKVcfg := &memberlist.KVConfig{}
	flagext.DefaultValues(memberlistKVcfg)
	memberlistKVcfg.Codecs = []codec.Codec{
		ring.GetCodec(),
	}
	memberlistKVcfg.ClusterLabel = cfg.MemberlistClusterLabel
	memberlistKVcfg.ClusterLabelVerificationDisabled = cfg.MemberlistClusterLabelVerificationDisabled
	if cfg.MemberlistBindAddr != "" {
		memberlistKVcfg.TCPTransport.BindAddrs = []string{cfg.MemberlistBindAddr}
	}
	if cfg.MemberlistAdvertiseAddr != "" {
		memberlistKVcfg.AdvertiseAddr = cfg.MemberlistAdvertiseAddr
	}
	memberlistKVcfg.JoinMembers = []string{cfg.MemberlistJoinMember}

	return memberlistKVcfg
}
