// Пакет csv — общий адаптер CSV, который рулится column-map. Про конкретный фид
// он ничего не знает: номера колонок приходят из настройки источника.
package csv

import (
	"context"

	"github.com/coffeinium/chaff/internal/feed"
	"github.com/coffeinium/chaff/internal/kernel"
	"github.com/coffeinium/chaff/internal/model"
)

func init() {
	kernel.Register("feed-csv", func() kernel.Module { return &Module{} })
}

type Module struct {
	k *kernel.Kernel
}

func (m *Module) Name() string                    { return "feed-csv" }
func (m *Module) Needs() []string                 { return nil }
func (m *Module) Init(k *kernel.Kernel) error     { m.k = k; return nil }
func (m *Module) Start(ctx context.Context) error { return nil }
func (m *Module) Stop(ctx context.Context) error  { return nil }
func (m *Module) Health() kernel.Health {
	return kernel.Health{OK: true, Detail: "общий CSV-адаптер"}
}

func (m *Module) Adapter() string { return "csv" }

// Fetch читает CSV и раскладывает колонки. Ключи column_map: indicator (по
// умолчанию 0), type, threat, note. Нет колонки type — вид определяется сам.
func (m *Module) Fetch(ctx context.Context, spec model.SourceSpec) ([]model.Indicator, error) {
	raw, err := feed.ReadURI(ctx, spec.URI)
	if err != nil {
		return nil, err
	}
	rows, err := feed.ParseCSV(raw, true)
	if err != nil {
		return nil, err
	}

	cm := spec.ColumnMap
	valueCol := col(cm, "indicator", 0)
	typeCol, hasType := cm["type"]
	threatCol, hasThreat := cm["threat"]
	noteCol, hasNote := cm["note"]

	out := make([]model.Indicator, 0, len(rows))
	for _, rec := range rows {
		value := feed.Col(rec, valueCol)
		if value == "" {
			continue
		}
		kind := model.Classify(value)
		if hasType {
			kind = model.NormalizeKind(feed.Col(rec, typeCol), value)
		}
		ind := model.Indicator{Value: value, Kind: kind, Action: model.ActionBlock, Scope: model.ScopeDomain}
		if hasThreat {
			ind.Threat = feed.Col(rec, threatCol)
		}
		if hasNote {
			ind.Note = feed.Col(rec, noteCol)
		}
		out = append(out, ind)
	}
	return out, nil
}

func col(cm map[string]int, key string, def int) int {
	if v, ok := cm[key]; ok {
		return v
	}
	return def
}
