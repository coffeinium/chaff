// Пакет ipblock блокирует индикаторы ip/cidr в ядре через named set nftables
// (атомарный swap, fail-safe). Это Enforcer.
//
// Заглушка: пока считает желаемый set; программирование nftables — после verify.
package ipblock

import (
	"context"
	"sync"

	"github.com/coffeinium/chaff/internal/kernel"
	"github.com/coffeinium/chaff/internal/model"
)

func init() {
	kernel.Register("ipblock", func() kernel.Module { return &Module{} })
}

type Module struct {
	k  *kernel.Kernel
	mu sync.Mutex
	v4 int
	v6 int
}

func (m *Module) Name() string    { return "ipblock" }
func (m *Module) Needs() []string { return []string{"bridge"} }
func (m *Module) Init(k *kernel.Kernel) error {
	m.k = k
	return nil
}
func (m *Module) Start(ctx context.Context) error { return nil }
func (m *Module) Stop(ctx context.Context) error  { return nil }

func (m *Module) Handles() []model.Kind {
	return []model.Kind{model.KindIP, model.KindCIDR}
}

// Enforce здесь будет программировать set blocklist_v4/v6. Пока лишь запоминает
// желаемые размеры, чтобы status/apply имели смысл end-to-end.
func (m *Module) Enforce(snap model.Ruleset) error {
	m.mu.Lock()
	m.v4, m.v6 = len(snap.IPv4), len(snap.IPv6)
	m.mu.Unlock()
	m.k.Log.Info("ipblock: enforce (заглушка)", "v4", len(snap.IPv4), "v6", len(snap.IPv6), "allow", len(snap.Allow.IPs))
	return nil
}

func (m *Module) Health() kernel.Health {
	m.mu.Lock()
	defer m.mu.Unlock()
	return kernel.Health{
		OK:      true,
		Detail:  "заглушка (nftables ждёт verify)",
		Metrics: map[string]any{"ipv4": m.v4, "ipv6": m.v6},
	}
}
