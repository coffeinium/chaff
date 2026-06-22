package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	activeTab   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("39")).Padding(0, 1)
	inactiveTab = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Padding(0, 1)
	cursorStyle = lipgloss.NewStyle().Bold(true)
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	okStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	offStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true)
	helpStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

func (m appModel) View() string {
	return m.header() + "\n\n" + m.body() + "\n" + m.footer()
}

func (m appModel) header() string {
	tabs := make([]string, len(viewTitles))
	for i, t := range viewTitles {
		label := fmt.Sprintf("%d %s", i+1, t)
		if viewID(i) == m.view {
			tabs[i] = activeTab.Render(label)
		} else {
			tabs[i] = inactiveTab.Render(label)
		}
	}
	return titleStyle.Render("chaff") + "  " + strings.Join(tabs, " ")
}

func (m appModel) body() string {
	switch m.view {
	case viewStatus:
		return m.bodyStatus()
	case viewModules:
		return m.bodyModules()
	case viewSources:
		return m.bodySources()
	case viewIndicators:
		return m.bodyIndicators()
	}
	return ""
}

func (m appModel) bodyStatus() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Индикаторы") + "\n")
	if len(m.status.Indicators) == 0 {
		b.WriteString(dimStyle.Render("  нет данных (r — обновить)") + "\n")
	} else {
		keys := make([]string, 0, len(m.status.Indicators))
		for k := range m.status.Indicators {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			b.WriteString(fmt.Sprintf("  %-8s %d\n", k, m.status.Indicators[k]))
		}
	}
	running := 0
	for _, mod := range m.status.Modules {
		if mod.Running {
			running++
		}
	}
	b.WriteString("\n" + titleStyle.Render("Модули") + "\n")
	b.WriteString(fmt.Sprintf("  запущено %d из %d\n", running, len(m.status.Modules)))
	return b.String()
}

func (m appModel) bodyModules() string {
	if len(m.modules) == 0 {
		return dimStyle.Render("  нет данных (r — обновить)")
	}
	start, end := m.window(len(m.modules))
	var b strings.Builder
	for i := start; i < end; i++ {
		row := m.modules[i]
		state := offStyle.Render("off")
		if row.Enabled {
			state = okStyle.Render("on ")
		}
		run := dimStyle.Render("·")
		if row.Running {
			run = okStyle.Render("●")
		}
		line := fmt.Sprintf("%s%-12s %s %s %s", caret(i == m.cursor), row.Name, state, run, dimStyle.Render(row.Health.Detail))
		b.WriteString(emph(i == m.cursor, line) + "\n")
	}
	return b.String()
}

func (m appModel) bodySources() string {
	if len(m.sources) == 0 {
		return dimStyle.Render("  фидов нет — добавь: chaff source add ...")
	}
	start, end := m.window(len(m.sources))
	var b strings.Builder
	for i := start; i < end; i++ {
		s := m.sources[i]
		state := offStyle.Render("off")
		if s.Enabled {
			state = okStyle.Render("on ")
		}
		line := fmt.Sprintf("%s%-14s %-7s %s %s", caret(i == m.cursor), s.Name, s.Adapter, state, dimStyle.Render(truncate(s.URI, 40)))
		b.WriteString(emph(i == m.cursor, line) + "\n")
	}
	return b.String()
}

func (m appModel) bodyIndicators() string {
	kind := indKinds[m.indKind]
	var b strings.Builder
	b.WriteString(titleStyle.Render("Вид: "+string(kind)) + dimStyle.Render("   (←→ сменить)") + "\n")
	if len(m.inds) == 0 {
		b.WriteString(dimStyle.Render("  пусто"))
		return b.String()
	}
	start, end := m.window(len(m.inds))
	for i := start; i < end; i++ {
		in := m.inds[i]
		line := fmt.Sprintf("%s%-46s %s", caret(i == m.cursor), truncate(in.Value, 46), dimStyle.Render(string(in.Action)))
		b.WriteString(emph(i == m.cursor, line) + "\n")
	}
	b.WriteString(dimStyle.Render(fmt.Sprintf("  %d/%d", m.cursor+1, len(m.inds))))
	return b.String()
}

func (m appModel) footer() string {
	keys := "tab/1-4 экран · r обновить · q выход"
	switch m.view {
	case viewModules:
		keys = "↑↓ выбор · space вкл/выкл · " + keys
	case viewSources:
		keys = "↑↓ выбор · s синк · " + keys
	case viewIndicators:
		keys = "←→ вид · ↑↓ прокрутка · " + keys
	}
	out := helpStyle.Render(keys)
	if m.err != "" {
		out += "\n" + errStyle.Render("ошибка: "+m.err)
	} else if m.msg != "" {
		out += "\n" + dimStyle.Render(m.msg)
	}
	return out
}

// window выбирает видимый диапазон строк так, чтобы курсор был на экране.
func (m appModel) window(total int) (int, int) {
	h := m.height - 8
	if h < 3 {
		h = 3
	}
	if total <= h {
		return 0, total
	}
	start := m.cursor - h/2
	if start < 0 {
		start = 0
	}
	if start > total-h {
		start = total - h
	}
	return start, start + h
}

func caret(selected bool) string {
	if selected {
		return "› "
	}
	return "  "
}

func emph(selected bool, s string) string {
	if selected {
		return cursorStyle.Render(s)
	}
	return s
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return string(r[:n])
	}
	return string(r[:n-1]) + "…"
}
