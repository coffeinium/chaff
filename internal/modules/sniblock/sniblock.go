// Пакет sniblock блокирует индикаторы domain/url инлайн: берёт SNI из TLS
// ClientHello и Host/URL из обычного HTTP через NFQUEUE и выносит вердикт по
// соединению. allow выигрывает; path-правила для HTTPS — только monitor, без
// расшифровки TLS пути не видно. Это Enforcer.
//
// Заглушка: пока считает желаемые правила; NFQUEUE+SNI — после verify.
package sniblock

import (
	"context"
	"sync"

	"github.com/coffeinium/chaff/internal/kernel"
	"github.com/coffeinium/chaff/internal/model"
)

func init() {
	kernel.Register("sniblock", func() kernel.Module { return &Module{} })
}

type Module struct {
	k       *kernel.Kernel
	mu      sync.Mutex
	domains int
	urls    int
}

func (m *Module) Name() string    { return "sniblock" }
func (m *Module) Needs() []string { return []string{"bridge"} }
func (m *Module) Init(k *kernel.Kernel) error {
	m.k = k
	return nil
}
func (m *Module) Start(ctx context.Context) error { return nil }
func (m *Module) Stop(ctx context.Context) error  { return nil }

func (m *Module) Handles() []model.Kind {
	return []model.Kind{model.KindDomain, model.KindURL}
}

func (m *Module) Enforce(snap model.Ruleset) error {
	m.mu.Lock()
	m.domains, m.urls = len(snap.Domains), len(snap.URLs)
	m.mu.Unlock()
	m.k.Log.Info("sniblock: enforce (заглушка)", "domains", len(snap.Domains), "urls", len(snap.URLs), "allow", len(snap.Allow.Domains))
	return nil
}

func (m *Module) Health() kernel.Health {
	m.mu.Lock()
	defer m.mu.Unlock()
	return kernel.Health{
		OK:      true,
		Detail:  "заглушка (NFQUEUE/SNI ждёт verify)",
		Metrics: map[string]any{"domains": m.domains, "urls": m.urls},
	}
}
