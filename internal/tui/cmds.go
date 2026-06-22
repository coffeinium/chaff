package tui

import (
	"encoding/json"
	"errors"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/coffeinium/chaff/internal/ipc"
	"github.com/coffeinium/chaff/internal/model"
)

type healthDTO struct {
	OK      bool           `json:"ok"`
	Detail  string         `json:"detail"`
	Metrics map[string]any `json:"metrics"`
}

type moduleRow struct {
	Name    string    `json:"name"`
	Title   string    `json:"title"`
	About   string    `json:"about"`
	Enabled bool      `json:"enabled"`
	Running bool      `json:"running"`
	Health  healthDTO `json:"health"`
}

type statusDTO struct {
	Modules    []moduleRow    `json:"modules"`
	Indicators map[string]int `json:"indicators"`
}

type (
	errMsg        struct{ err error }
	statusMsg     struct{ s statusDTO }
	modulesMsg    struct{ rows []moduleRow }
	sourcesMsg    struct{ rows []model.SourceSpec }
	indicatorsMsg struct{ rows []model.Indicator }
	actionMsg     struct{ text string }
)

func decode(data, target any) error {
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, target)
}

func request(socket string, req ipc.Request) (ipc.Response, error) {
	resp, err := ipc.Call(socket, req)
	if err != nil {
		return resp, err
	}
	if !resp.OK {
		return resp, errors.New(resp.Error)
	}
	return resp, nil
}

func fetchStatus(socket string) tea.Cmd {
	return func() tea.Msg {
		resp, err := request(socket, ipc.Request{Cmd: "status"})
		if err != nil {
			return errMsg{err}
		}
		var s statusDTO
		if err := decode(resp.Data, &s); err != nil {
			return errMsg{err}
		}
		return statusMsg{s}
	}
}

func fetchModules(socket string) tea.Cmd {
	return func() tea.Msg {
		resp, err := request(socket, ipc.Request{Cmd: "module.ls"})
		if err != nil {
			return errMsg{err}
		}
		var rows []moduleRow
		if err := decode(resp.Data, &rows); err != nil {
			return errMsg{err}
		}
		return modulesMsg{rows}
	}
}

func fetchSources(socket string) tea.Cmd {
	return func() tea.Msg {
		resp, err := request(socket, ipc.Request{Cmd: "source.ls"})
		if err != nil {
			return errMsg{err}
		}
		var rows []model.SourceSpec
		if err := decode(resp.Data, &rows); err != nil {
			return errMsg{err}
		}
		return sourcesMsg{rows}
	}
}

func fetchIndicators(socket string, kind model.Kind) tea.Cmd {
	return func() tea.Msg {
		resp, err := request(socket, ipc.Request{Cmd: "list", Args: map[string]string{"kind": string(kind)}})
		if err != nil {
			return errMsg{err}
		}
		var rows []model.Indicator
		if err := decode(resp.Data, &rows); err != nil {
			return errMsg{err}
		}
		return indicatorsMsg{rows}
	}
}

func toggleModule(socket, name string, enable bool) tea.Cmd {
	return func() tea.Msg {
		cmd := "module.disable"
		if enable {
			cmd = "module.enable"
		}
		resp, err := request(socket, ipc.Request{Cmd: cmd, Args: map[string]string{"name": name}})
		if err != nil {
			return errMsg{err}
		}
		text, _ := resp.Data.(string)
		return actionMsg{text}
	}
}

func syncSources(socket string) tea.Cmd {
	return func() tea.Msg {
		resp, err := request(socket, ipc.Request{Cmd: "source.sync"})
		if err != nil {
			return errMsg{err}
		}
		text, _ := resp.Data.(string)
		return actionMsg{text}
	}
}
