package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/coffeinium/chaff/internal/model"
)

type viewID int

const (
	viewStatus viewID = iota
	viewModules
	viewSources
	viewIndicators
	viewCount
)

var viewTitles = []string{"Статус", "Функции", "Списки", "Блокировки"}

var indKinds = []model.Kind{
	model.KindIP, model.KindCIDR, model.KindDomain,
	model.KindURL, model.KindSHA256, model.KindMD5,
}

type appModel struct {
	socket string
	view   viewID
	cursor int

	status  statusDTO
	modules []moduleRow
	sources []model.SourceSpec
	inds    []model.Indicator
	indKind int

	msg string
	err string

	width  int
	height int
}

func Run(socket string) error {
	p := tea.NewProgram(appModel{socket: socket}, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func (m appModel) Init() tea.Cmd { return fetchStatus(m.socket) }

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	case errMsg:
		m.err = msg.err.Error()
		return m, nil
	case statusMsg:
		m.status, m.err = msg.s, ""
		return m, nil
	case modulesMsg:
		m.modules, m.err = msg.rows, ""
		m.clampCursor(len(m.modules))
		return m, nil
	case sourcesMsg:
		m.sources, m.err = msg.rows, ""
		m.clampCursor(len(m.sources))
		return m, nil
	case indicatorsMsg:
		m.inds, m.err = msg.rows, ""
		m.clampCursor(len(m.inds))
		return m, nil
	case actionMsg:
		m.msg = msg.text
		return m, m.load()
	}
	return m, nil
}

func (m appModel) handleKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "tab":
		m.view = (m.view + 1) % viewCount
		m.cursor, m.msg = 0, ""
		return m, m.load()
	case "shift+tab":
		m.view = (m.view + viewCount - 1) % viewCount
		m.cursor, m.msg = 0, ""
		return m, m.load()
	case "1", "2", "3", "4":
		m.view = viewID(k.String()[0] - '1')
		m.cursor, m.msg = 0, ""
		return m, m.load()
	case "r":
		m.msg = ""
		return m, m.load()
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil
	case "down", "j":
		if m.cursor < m.rowCount()-1 {
			m.cursor++
		}
		return m, nil
	}

	switch m.view {
	case viewModules:
		if (k.String() == " " || k.String() == "enter") && m.cursor < len(m.modules) {
			row := m.modules[m.cursor]
			return m, toggleModule(m.socket, row.Name, !row.Enabled)
		}
	case viewSources:
		if k.String() == "s" || k.String() == "enter" {
			return m, syncSources(m.socket)
		}
	case viewIndicators:
		switch k.String() {
		case "left", "h":
			m.indKind = (m.indKind + len(indKinds) - 1) % len(indKinds)
			m.cursor = 0
			return m, fetchIndicators(m.socket, indKinds[m.indKind])
		case "right", "l":
			m.indKind = (m.indKind + 1) % len(indKinds)
			m.cursor = 0
			return m, fetchIndicators(m.socket, indKinds[m.indKind])
		}
	}
	return m, nil
}

func (m appModel) load() tea.Cmd {
	switch m.view {
	case viewStatus:
		return fetchStatus(m.socket)
	case viewModules:
		return fetchModules(m.socket)
	case viewSources:
		return fetchSources(m.socket)
	case viewIndicators:
		return fetchIndicators(m.socket, indKinds[m.indKind])
	}
	return nil
}

func (m appModel) rowCount() int {
	switch m.view {
	case viewModules:
		return len(m.modules)
	case viewSources:
		return len(m.sources)
	case viewIndicators:
		return len(m.inds)
	}
	return 0
}

func (m *appModel) clampCursor(n int) {
	if m.cursor >= n {
		m.cursor = n - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}
