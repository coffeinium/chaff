// Пакет kernel — микроядро: держит общие сервисы (config, log, store, bus) и
// жизненный цикл модулей. Про nftables, DNS и форматы фидов оно не знает — это
// всё в модулях. Выключить фичу = выключить её модуль; ядро и соседи живут дальше.
package kernel

import (
	"context"

	"github.com/coffeinium/chaff/internal/model"
)

// Health — самоотчёт модуля, который показывает chaff status.
type Health struct {
	OK      bool           `json:"ok"`
	Detail  string         `json:"detail,omitempty"`
	Metrics map[string]any `json:"metrics,omitempty"`
}

// Module — единица функциональности. В chaff любая возможность — это модуль.
type Module interface {
	// Name — уникальный id в реестре, например "sniblock".
	Name() string
	// Needs — модули, которые должны стартовать раньше. Зависимости поднимаются
	// автоматически, даже если их явно не включали.
	Needs() []string
	// Init — связать общие сервисы из ядра. Фоновую работу тут не начинаем.
	Init(k *Kernel) error
	// Start — запустить фоновую работу; возвращаться должен быстро (горутины).
	Start(ctx context.Context) error
	// Stop — остановиться; зовётся в обратном порядке старта.
	Stop(ctx context.Context) error
	// Health — текущее состояние для status.
	Health() Health
}

// Enforcer — необязательная способность: модуль, который из желаемого Ruleset
// программирует data-plane. apply/Reconcile кормит им каждый включённый энфорсер.
type Enforcer interface {
	Module
	// Handles — какие виды индикаторов этот энфорсер берёт на себя (справочно;
	// сам он читает из Ruleset понятные ему части).
	Handles() []model.Kind
	// Enforce приводит data-plane к снапшоту snap. Должен быть атомарным и
	// fail-safe: на ошибке держим последнее рабочее состояние, а не открываемся.
	Enforce(snap model.Ruleset) error
}

// Source — необязательная способность: адаптер фида, отдающий индикаторы. Один
// адаптер обслуживает много настроенных SourceSpec.
type Source interface {
	Module
	// Adapter — значение spec.Adapter, которое этот модуль обслуживает ("csv").
	Adapter() string
	Fetch(ctx context.Context, spec model.SourceSpec) ([]model.Indicator, error)
}
