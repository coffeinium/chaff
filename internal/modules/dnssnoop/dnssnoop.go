// Пакет dnssnoop пассивно читает DNS-ответы, идущие сквозь мост (read-only,
// ничего не редиректит — резолвером остаётся вышестоящий роутер) и кладёт связки
// домен->ip в кэш snoop, отдавая найденные bad-IP в ipblock.
//
// Заглушка: снифинг появится после verify.
package dnssnoop

import (
	"context"

	"github.com/coffeinium/chaff/internal/kernel"
)

func init() {
	kernel.Register("dnssnoop", func() kernel.Module { return &Module{} })
}

type Module struct {
	k *kernel.Kernel
}

func (m *Module) Name() string    { return "dnssnoop" }
func (m *Module) Needs() []string { return []string{"bridge"} }
func (m *Module) Init(k *kernel.Kernel) error {
	m.k = k
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	m.k.Log.Info("dnssnoop: заглушка (пассивный DNS-снуп ещё не работает)")
	return nil
}

func (m *Module) Stop(ctx context.Context) error { return nil }

func (m *Module) Health() kernel.Health {
	return kernel.Health{OK: true, Detail: "заглушка (пассивный DNS ждёт verify)"}
}
