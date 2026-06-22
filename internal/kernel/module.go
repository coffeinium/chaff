package kernel

import (
	"context"

	"github.com/coffeinium/chaff/internal/model"
)

type Health struct {
	OK      bool           `json:"ok"`
	Detail  string         `json:"detail,omitempty"`
	Metrics map[string]any `json:"metrics,omitempty"`
}

type Module interface {
	Name() string

	Needs() []string

	Init(k *Kernel) error

	Start(ctx context.Context) error

	Stop(ctx context.Context) error

	Health() Health
}

type Enforcer interface {
	Module

	Handles() []model.Kind

	Enforce(snap model.Ruleset) error
}

type Describer interface {
	Title() string
	About() string
}

type Source interface {
	Module

	Adapter() string
	Fetch(ctx context.Context, spec model.SourceSpec) ([]model.Indicator, error)
}
