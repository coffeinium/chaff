// Пакет fstec — CSV-адаптер, заранее заточенный под раскладку FSTEC IOC
// (indicator,type,threat,punkt,review,note). Это рабочий пример фид-специфичного
// адаптера; сам движок остаётся format-agnostic. Флаг review помечает
// легит-инфру: такие домены/URL становятся monitor + scope path (домен целиком
// блокировать нельзя).
package fstec

import (
	"context"

	"github.com/coffeinium/chaff/internal/feed"
	"github.com/coffeinium/chaff/internal/kernel"
	"github.com/coffeinium/chaff/internal/model"
)

func init() {
	kernel.Register("feed-fstec", func() kernel.Module { return &Module{} })
}

const (
	colIndicator = 0
	colType      = 1
	colThreat    = 2
	colReview    = 4
	colNote      = 5
)

type Module struct {
	k *kernel.Kernel
}

func (m *Module) Name() string                    { return "feed-fstec" }
func (m *Module) Needs() []string                 { return nil }
func (m *Module) Init(k *kernel.Kernel) error     { m.k = k; return nil }
func (m *Module) Start(ctx context.Context) error { return nil }
func (m *Module) Stop(ctx context.Context) error  { return nil }
func (m *Module) Health() kernel.Health {
	return kernel.Health{OK: true, Detail: "адаптер FSTEC CSV (пример)"}
}

func (m *Module) Adapter() string { return "fstec" }

func (m *Module) Fetch(ctx context.Context, spec model.SourceSpec) ([]model.Indicator, error) {
	raw, err := feed.ReadURI(ctx, spec.URI)
	if err != nil {
		return nil, err
	}
	rows, err := feed.ParseCSV(raw, true)
	if err != nil {
		return nil, err
	}

	out := make([]model.Indicator, 0, len(rows))
	for _, rec := range rows {
		value := feed.Col(rec, colIndicator)
		if value == "" {
			continue
		}
		kind := model.NormalizeKind(feed.Col(rec, colType), value)
		ind := model.Indicator{
			Value:  value,
			Kind:   kind,
			Action: model.ActionBlock,
			Scope:  model.ScopeDomain,
			Threat: feed.Col(rec, colThreat),
			Note:   feed.Col(rec, colNote),
		}
		// Легит-инфра (review=1): домен целиком не блокируем. Для URL держим
		// путь, но только monitor — без расшифровки TLS путь в HTTPS не видно.
		if feed.Col(rec, colReview) == "1" && (kind == model.KindDomain || kind == model.KindURL) {
			ind.Action = model.ActionMonitor
			if kind == model.KindURL {
				ind.Scope = model.ScopePath
			} else {
				ind.Action = model.ActionAllow // легит-домен целиком оставляем живым
			}
		}
		out = append(out, ind)
	}
	return out, nil
}
