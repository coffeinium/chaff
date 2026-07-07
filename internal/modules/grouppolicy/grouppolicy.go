// Package grouppolicy — ОПАСНЫЙ ЭКСПЕРИМЕНТАЛЬНЫЙ модуль групповых политик.
//
// Модуль по умолчанию выключен. При включении открывается раздел «Группы» в
// веб-панели и команды `chaff group ...` в CLI. Группа — это набор машин
// (по MAC или имени хоста) с общей политикой block/allow, которая применяется к
// участникам, когда группа включена.
//
// Приоритет решений (сверху вниз): ручное правило (block/allow) → групповое →
// список-фид. То есть текущие ручные («фундаментальные») блокировки всегда
// перевешивают групповые правила, а групповое правило, которое им противоречит,
// запрещено создавать. Одна машина не может состоять больше чем в одной группе.
package grouppolicy

import (
	"context"

	"github.com/coffeinium/chaff/internal/kernel"
)

func init() {
	kernel.Register("grouppolicy", func() kernel.Module { return &Module{} })
}

type Module struct {
	k *kernel.Kernel
}

func (m *Module) Name() string { return "grouppolicy" }

// Групповая политика применяется через MAC-блокировки, поэтому нужен macblock.
func (m *Module) Needs() []string { return []string{"macblock"} }

func (m *Module) Title() string { return "Групповые политики" }
func (m *Module) About() string {
	return "ОПАСНО · ЭКСПЕРИМЕНТ: блокировка/разблокировка машин группами по MAC/имени"
}

func (m *Module) Init(k *kernel.Kernel) error     { m.k = k; return nil }
func (m *Module) Start(ctx context.Context) error { return nil }
func (m *Module) Stop(ctx context.Context) error  { return nil }

func (m *Module) Health() kernel.Health {
	groups, err := m.k.Store.ListGroups()
	if err != nil {
		return kernel.Health{OK: false, Detail: "ошибка: " + err.Error()}
	}
	on, machines := 0, 0
	for _, g := range groups {
		if g.Enabled {
			on++
		}
		machines += len(g.Members)
	}
	met := map[string]any{"групп": len(groups), "включено": on, "участников": machines}
	return kernel.Health{
		OK:      true,
		Detail:  "ЭКСПЕРИМЕНТ · ручные блокировки в приоритете",
		Metrics: met,
	}
}
