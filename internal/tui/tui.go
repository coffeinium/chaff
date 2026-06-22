package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/coffeinium/chaff/internal/model"
)

type viewID int

const (
	viewStatus viewID = iota
	viewHits
	viewModules
	viewSources
	viewIndicators
	viewCount
)

var viewTitles = []string{"Статус", "Срабатывания", "Функции", "Списки", "Блокировки"}

var indKinds = []model.Kind{
	model.KindIP, model.KindCIDR, model.KindDomain,
	model.KindURL, model.KindSHA256, model.KindMD5,
}

const refreshEvery = 3 * time.Second

type appModel struct {
	socket string
	view   viewID
	cursor int

	status  statusDTO
	modules []moduleRow
	sources []model.SourceSpec
	inds    []model.Indicator
	hits    []hitRow
	indKind int

	search    bool
	query     string
	showHelp  bool
	auto      bool
	action    bool
	actionVal string

	msg string
	err string

	width  int
	height int
}

type tickMsg struct{}

func Run(socket string) error {
	p := tea.NewProgram(appModel{socket: socket, auto: true}, tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func (m appModel) Init() tea.Cmd { return tea.Batch(m.refresh(), tick()) }

func tick() tea.Cmd {
	return tea.Tick(refreshEvery, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m appModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyMsg:
		return m.handleKey(msg)
	case tickMsg:
		if m.auto && !m.search && !m.showHelp && !m.action {
			return m, tea.Batch(m.refresh(), tick())
		}
		return m, tick()
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
		m.clampCursor(m.rowCount())
		return m, nil
	case hitsMsg:
		m.hits, m.err = msg.rows, ""
		m.clampCursor(len(m.hits))
		return m, nil
	case actionMsg:
		m.msg = msg.text
		return m, m.refresh()
	}
	return m, nil
}

func (m appModel) handleKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.showHelp {
		if s := k.String(); s == "q" || s == "ctrl+c" {
			return m, tea.Quit
		}
		m.showHelp = false
		return m, nil
	}
	if m.search {
		return m.handleSearchKey(k)
	}
	if m.action {
		return m.handleActionKey(k)
	}

	switch k.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "?":
		m.showHelp = true
		return m, nil
	case "a":
		m.auto = !m.auto
		return m, nil
	case "tab":
		m.view = (m.view + 1) % viewCount
		m.resetView()
		return m, m.refresh()
	case "shift+tab":
		m.view = (m.view + viewCount - 1) % viewCount
		m.resetView()
		return m, m.refresh()
	case "r":
		m.msg = ""
		return m, m.refresh()
	case "up", "k":
		m.moveCursor(-1)
		return m, nil
	case "down", "j":
		m.moveCursor(1)
		return m, nil
	case "esc":
		if m.query != "" {
			m.query, m.cursor = "", 0
		}
		return m, nil
	}

	if s := k.String(); len(s) == 1 && s[0] >= '1' && s[0] <= '9' {
		if n := viewID(s[0] - '1'); n < viewCount {
			m.view = n
			m.resetView()
			return m, m.refresh()
		}
	}

	switch m.view {
	case viewModules:
		if (k.String() == " " || k.String() == "enter") && m.cursor < len(m.modules) {
			row := m.modules[m.cursor]
			return m, toggleModule(m.socket, row.Name, !row.Enabled)
		}
	case viewSources:
		switch k.String() {
		case "s", "enter":
			return m, syncSources(m.socket)
		case " ":
			if m.cursor < len(m.sources) {
				s := m.sources[m.cursor]
				return m, toggleSource(m.socket, s.ID, !s.Enabled)
			}
		}
	case viewHits:
		if k.String() == "enter" && m.cursor < len(m.hits) {
			m.action, m.actionVal = true, m.hits[m.cursor].Indicator
			return m, nil
		}
	case viewIndicators:
		switch k.String() {
		case "left", "h":
			m.indKind = (m.indKind + len(indKinds) - 1) % len(indKinds)
			m.cursor, m.query = 0, ""
			return m, fetchIndicators(m.socket, indKinds[m.indKind])
		case "right", "l":
			m.indKind = (m.indKind + 1) % len(indKinds)
			m.cursor, m.query = 0, ""
			return m, fetchIndicators(m.socket, indKinds[m.indKind])
		case "/":
			m.search = true
			return m, nil
		case "enter":
			inds := m.filteredInds()
			if m.cursor < len(inds) {
				m.action, m.actionVal = true, inds[m.cursor].Value
			}
			return m, nil
		}
	}
	return m, nil
}

func (m appModel) handleActionKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc", "q":
		m.action = false
		return m, nil
	case "b":
		m.action = false
		return m, manualAction(m.socket, "block.add", m.actionVal)
	case "w":
		m.action = false
		return m, manualAction(m.socket, "allow.add", m.actionVal)
	case "d":
		m.action = false
		return m, manualAction(m.socket, "block.rm", m.actionVal)
	}
	return m, nil
}

func (m appModel) handleSearchKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "enter":
		m.search = false
		return m, nil
	case "esc":
		m.search, m.query, m.cursor = false, "", 0
		return m, nil
	case "backspace":
		if r := []rune(m.query); len(r) > 0 {
			m.query, m.cursor = string(r[:len(r)-1]), 0
		}
		return m, nil
	case "up":
		m.moveCursor(-1)
		return m, nil
	case "down":
		m.moveCursor(1)
		return m, nil
	}
	if k.Type == tea.KeyRunes {
		m.query += string(k.Runes)
		m.cursor = 0
	}
	return m, nil
}

func (m appModel) refresh() tea.Cmd {
	cmds := []tea.Cmd{fetchStatus(m.socket)}
	if c := m.load(); c != nil {
		cmds = append(cmds, c)
	}
	return tea.Batch(cmds...)
}

func (m appModel) load() tea.Cmd {
	switch m.view {
	case viewModules:
		return fetchModules(m.socket)
	case viewSources:
		return fetchSources(m.socket)
	case viewIndicators:
		return fetchIndicators(m.socket, indKinds[m.indKind])
	case viewHits:
		return fetchHits(m.socket, 200)
	}
	return nil
}

func (m *appModel) resetView() {
	m.cursor, m.msg, m.query, m.search = 0, "", "", false
}

func (m appModel) filteredInds() []model.Indicator {
	if m.query == "" {
		return m.inds
	}
	q := strings.ToLower(m.query)
	var out []model.Indicator
	for _, in := range m.inds {
		if strings.Contains(strings.ToLower(in.Value), q) {
			out = append(out, in)
		}
	}
	return out
}

func (m appModel) rowCount() int {
	switch m.view {
	case viewModules:
		return len(m.modules)
	case viewSources:
		return len(m.sources)
	case viewIndicators:
		return len(m.filteredInds())
	case viewHits:
		return len(m.hits)
	}
	return 0
}

func (m *appModel) moveCursor(d int) {
	n := m.rowCount()
	if n == 0 {
		m.cursor = 0
		return
	}
	m.cursor += d
	if m.cursor >= n {
		m.cursor = n - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}

func (m *appModel) clampCursor(n int) {
	if m.cursor >= n {
		m.cursor = n - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
}
