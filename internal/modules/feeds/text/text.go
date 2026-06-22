package text

import (
	"context"

	"github.com/coffeinium/chaff/internal/feed"
	"github.com/coffeinium/chaff/internal/kernel"
	"github.com/coffeinium/chaff/internal/model"
)

func init() {
	kernel.Register("feed-text", func() kernel.Module { return &Module{} })
}

type Module struct {
	k *kernel.Kernel
}

func (m *Module) Name() string                    { return "feed-text" }
func (m *Module) Needs() []string                 { return nil }
func (m *Module) Title() string                   { return "Источник: список" }
func (m *Module) About() string                   { return "загружает список построчно" }
func (m *Module) Init(k *kernel.Kernel) error     { m.k = k; return nil }
func (m *Module) Start(ctx context.Context) error { return nil }
func (m *Module) Stop(ctx context.Context) error  { return nil }
func (m *Module) Health() kernel.Health {
	return kernel.Health{OK: true, Detail: "готов"}
}

func (m *Module) Adapter() string { return "text" }

func (m *Module) Fetch(ctx context.Context, spec model.SourceSpec) ([]model.Indicator, error) {
	raw, err := feed.ReadURI(ctx, spec.URI)
	if err != nil {
		return nil, err
	}
	var out []model.Indicator
	for _, line := range feed.Lines(raw) {
		kind := model.Classify(line)
		if kind == model.KindUnknown {
			continue
		}
		out = append(out, model.Indicator{
			Value: line, Kind: kind, Action: model.ActionBlock, Scope: model.ScopeDomain,
		})
	}
	return out, nil
}
