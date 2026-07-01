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

func (m *Module) Name() string    { return "feed-csv" }
func (m *Module) Needs() []string { return nil }
func (m *Module) Title() string   { return "Источник: CSV" }
func (m *Module) About() string {
	return "загружает список из CSV-файла или по ссылке"
}
func (m *Module) Init(k *kernel.Kernel) error     { m.k = k; return nil }
func (m *Module) Start(ctx context.Context) error { return nil }
func (m *Module) Stop(ctx context.Context) error  { return nil }
func (m *Module) Health() kernel.Health {
	return kernel.Health{OK: true, Detail: "готов"}
}

func (m *Module) Adapter() string { return "csv" }

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
	reviewCol, hasReview := cm["review"]

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
		if hasReview && feed.Col(rec, reviewCol) == "1" {
			ind.Action = reviewAction(kind)
		}
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

func reviewAction(k model.Kind) model.Action {
	if k == model.KindURL {
		return model.ActionMonitor
	}
	return model.ActionAllow
}

func col(cm map[string]int, key string, def int) int {
	if v, ok := cm[key]; ok {
		return v
	}
	return def
}
