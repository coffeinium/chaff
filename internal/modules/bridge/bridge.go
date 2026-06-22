// Пакет bridge — фундамент data-plane: прозрачный L2-мост в разрыв
// (между внешним и внутренним интерфейсами) с br_netfilter, нужными sysctl,
// базовым ruleset
// nftables и хуком NFQUEUE, к которому цепляются энфорсеры.
//
// Заглушка: реальная настройка появится после verify-прохода.
package bridge

import (
	"context"

	"github.com/coffeinium/chaff/internal/kernel"
)

func init() {
	kernel.Register("bridge", func() kernel.Module { return &Module{} })
}

type Module struct {
	k *kernel.Kernel
}

func (m *Module) Name() string    { return "bridge" }
func (m *Module) Needs() []string { return nil }
func (m *Module) Init(k *kernel.Kernel) error {
	m.k = k
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	m.k.Log.Info("bridge: заглушка (мост в разрыв ещё не поднимается)")
	return nil
}

func (m *Module) Stop(ctx context.Context) error { return nil }

func (m *Module) Health() kernel.Health {
	return kernel.Health{OK: true, Detail: "заглушка (data-plane ждёт verify)"}
}
