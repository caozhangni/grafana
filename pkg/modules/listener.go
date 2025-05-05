package modules

import (
	"context"
	"errors"

	"github.com/grafana/dskit/modules"
	"github.com/grafana/dskit/services"

	"github.com/grafana/grafana/pkg/infra/log"
)

var _ services.ManagerListener = (*serviceListener)(nil)

type serviceListener struct {
	log     log.Logger
	service *service
}

func newServiceListener(logger log.Logger, s *service) *serviceListener {
	return &serviceListener{log: logger, service: s}
}

func (l *serviceListener) Healthy() {
	l.log.Info("All modules healthy")
}

func (l *serviceListener) Stopped() {
	l.log.Info("All modules stopped")
}

// INFO: 用于被dskit的serviceManager在状态发生变化时调用
// INFO: 注意这里会告知是一个service
func (l *serviceListener) Failure(service services.Service) {
	// if any service fails, stop all services
	// INFO: 如果有任何服务失败,停止所有服务
	if err := l.service.Shutdown(context.Background(), service.FailureCase().Error()); err != nil {
		l.log.Error("Failed to stop all modules", "err", err)
	}

	// log which module failed
	for module, s := range l.service.serviceMap {
		if s == service {
			if errors.Is(service.FailureCase(), modules.ErrStopProcess) {
				l.log.Info("Received stop signal via return error", "module", module, "err", service.FailureCase())
			} else {
				l.log.Error("Module failed", "module", module, "err", service.FailureCase())
			}
			return
		}
	}

	l.log.Error("Module failed", "module", "unknown", "err", service.FailureCase())
}
