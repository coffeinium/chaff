package hosts

import (
	"context"
	"strings"

	"github.com/coffeinium/chaff/internal/feed"
	"github.com/coffeinium/chaff/internal/kernel"
	"github.com/coffeinium/chaff/internal/model"
)

func init() {
	kernel.Register("feed-hosts", func() kernel.Module { return &Module{} })
}

type Module struct {
	k *kernel.Kernel
}

func (m *Module) Name() string                    { return "feed-hosts" }
func (m *Module) Needs() []string                 { return nil }
func (m *Module) Title() string                   { return "Источник: hosts" }
func (m *Module) About() string                   { return "загружает список из hosts-файла" }
func (m *Module) Init(k *kernel.Kernel) error     { m.k = k; return nil }
func (m *Module) Start(ctx context.Context) error { return nil }
func (m *Module) Stop(ctx context.Context) error  { return nil }
func (m *Module) Health() kernel.Health {
	return kernel.Health{OK: true, Detail: "готов"}
}

func (m *Module) Adapter() string { return "hosts" }

func (m *Module) Fetch(ctx context.Context, spec model.SourceSpec) ([]model.Indicator, error) {
	raw, err := feed.ReadURI(ctx, spec.URI)
	if err != nil {
		return nil, err
	}
	var out []model.Indicator
	for _, line := range feed.Lines(raw) {
		fields := strings.Fields(line)
		var host string
		switch len(fields) {
		case 1:
			host = fields[0]
		default:
			host = fields[1]
		}
		if host == "" || host == "localhost" {
			continue
		}
		out = append(out, model.Indicator{
			Value: host, Kind: model.KindDomain, Action: model.ActionBlock, Scope: model.ScopeDomain,
		})
	}
	return out, nil
}
