package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15"))
	activeTab   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("39")).Padding(0, 1)
	inactiveTab = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Padding(0, 1)
	cursorStyle = lipgloss.NewStyle().Bold(true)
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	okStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	warnStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	offStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true)
	helpStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

func (m appModel) View() string {
	if m.showHelp {
		return m.helpScreen()
	}
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
	left := titleStyle.Render("chaff") + "  " + strings.Join(tabs, " ")
	right := m.bridgeBadge() + "  " + m.autoBadge()
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 2 {
		return left + "  " + right
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m appModel) bridgeBadge() string {
	b := m.status.Bridge
	switch {
	case !b.Running:
		return dimStyle.Render("мост ○")
	case b.Up:
		return okStyle.Render("мост ●")
	case !b.Configured:
		return warnStyle.Render("мост ◐")
	default:
		return offStyle.Render("мост ✕")
	}
}

func (m appModel) autoBadge() string {
	if m.auto {
		return dimStyle.Render("авто ⟳")
	}
	return dimStyle.Render("авто ⏸")
}

func (m appModel) body() string {
	switch m.view {
	case viewStatus:
		return m.bodyStatus()
	case viewHits:
		return m.bodyHits()
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
	b.WriteString(titleStyle.Render("Состояние") + "\n")
	b.WriteString("  мост      " + m.bridgeLine() + "\n")
	running := 0
	for _, mod := range m.status.Modules {
		if mod.Running {
			running++
		}
	}
	b.WriteString(fmt.Sprintf("  функции   работает %d из %d\n", running, len(m.status.Modules)))

	b.WriteString("\n" + titleStyle.Render("Индикаторы по видам") + "\n")
	if len(m.status.Indicators) == 0 {
		b.WriteString(dimStyle.Render("  пусто — добавь источник: chaff source add … затем sync") + "\n")
		return b.String()
	}
	keys := make([]string, 0, len(m.status.Indicators))
	for k := range m.status.Indicators {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		b.WriteString(fmt.Sprintf("  %-8s %d\n", k, m.status.Indicators[k]))
	}
	return b.String()
}

func (m appModel) bridgeLine() string {
	b := m.status.Bridge
	switch {
	case !b.Running:
		return dimStyle.Render("выключен (модуль bridge не работает)")
	case b.Up:
		return okStyle.Render(b.Detail)
	case !b.Configured:
		return warnStyle.Render(b.Detail)
	default:
		return offStyle.Render(b.Detail)
	}
}

func (m appModel) bodyHits() string {
	if len(m.hits) == 0 {
		return dimStyle.Render("  срабатываний нет.\n  события пишет «Блокировка по сайтам» при разрыве по имени;\n  блок по IP режется в ядре и сюда не попадает.")
	}
	start, end := m.window(len(m.hits))
	var b strings.Builder
	for i := start; i < end; i++ {
		h := m.hits[i]
		when := time.Unix(h.TS, 0).Format("02.01 15:04:05")
		line := fmt.Sprintf("%s%s  %-4s  %-32s  %-15s  %s",
			caret(i == m.cursor), dimStyle.Render(when), h.Layer,
			truncate(h.Indicator, 32), h.SrcIP, actionStyled(h.Detail))
		b.WriteString(emph(i == m.cursor, line) + "\n")
	}
	b.WriteString(dimStyle.Render(fmt.Sprintf("  %d/%d", m.cursor+1, len(m.hits))))
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
		state := offStyle.Render("выкл")
		if row.Enabled {
			state = okStyle.Render("вкл ")
		}
		dot := dimStyle.Render("·")
		if row.Running {
			dot = okStyle.Render("●")
		}
		title := row.Title
		if title == "" {
			title = row.Name
		}
		line := fmt.Sprintf("%s%-22s %s %s %s", caret(i == m.cursor), title, state, dot, healthIcon(row))
		b.WriteString(emph(i == m.cursor, line) + "\n")
	}
	if sel := m.selectedModule(); sel != nil {
		b.WriteString("\n" + dimStyle.Render("  "+sel.About+"  ·  "+sel.Name))
		if sel.Health.Detail != "" {
			b.WriteString("\n" + dimStyle.Render("  здоровье: "+sel.Health.Detail))
		}
	}
	return b.String()
}

func healthIcon(row moduleRow) string {
	if !row.Running {
		return dimStyle.Render("—")
	}
	if row.Health.OK {
		return okStyle.Render("✓ ") + dimStyle.Render(row.Health.Detail)
	}
	return offStyle.Render("✕ " + row.Health.Detail)
}

func (m appModel) selectedModule() *moduleRow {
	if m.cursor >= 0 && m.cursor < len(m.modules) {
		return &m.modules[m.cursor]
	}
	return nil
}

func (m appModel) bodySources() string {
	if len(m.sources) == 0 {
		return dimStyle.Render("  списков нет — добавьте: chaff source add ...")
	}
	start, end := m.window(len(m.sources))
	var b strings.Builder
	for i := start; i < end; i++ {
		s := m.sources[i]
		state := offStyle.Render("выкл")
		if s.Enabled {
			state = okStyle.Render("вкл ")
		}
		line := fmt.Sprintf("%s%-14s %-7s %s %s", caret(i == m.cursor), s.Name, s.Adapter, state, dimStyle.Render(truncate(s.URI, 40)))
		b.WriteString(emph(i == m.cursor, line) + "\n")
	}
	return b.String()
}

func (m appModel) bodyIndicators() string {
	kind := indKinds[m.indKind]
	inds := m.filteredInds()
	var b strings.Builder
	b.WriteString(titleStyle.Render("Вид: "+string(kind)) + dimStyle.Render("   (←→ сменить · / поиск)") + "\n")
	if len(inds) == 0 {
		if m.query != "" {
			b.WriteString(dimStyle.Render("  ничего не найдено по «" + m.query + "»"))
		} else {
			b.WriteString(dimStyle.Render("  пусто"))
		}
		return b.String()
	}
	start, end := m.window(len(inds))
	for i := start; i < end; i++ {
		in := inds[i]
		line := fmt.Sprintf("%s%-46s %s", caret(i == m.cursor), truncate(in.Value, 46), actionStyled(string(in.Action)))
		b.WriteString(emph(i == m.cursor, line) + "\n")
	}
	b.WriteString(dimStyle.Render(fmt.Sprintf("  %d/%d", m.cursor+1, len(inds))))
	return b.String()
}

func actionStyled(a string) string {
	switch a {
	case "block":
		return offStyle.Render(a)
	case "monitor":
		return warnStyle.Render(a)
	case "allow":
		return okStyle.Render(a)
	}
	return dimStyle.Render(a)
}

func (m appModel) footer() string {
	if m.action {
		return helpStyle.Render("действие над ") + truncate(m.actionVal, 40) +
			helpStyle.Render(":  b заблокировать · w разрешить · d снять ручное · esc отмена")
	}
	if m.search {
		return helpStyle.Render("поиск: ") + m.query + helpStyle.Render("▏  enter — применить · esc — сброс")
	}
	base := "tab/1-5 экран · r обновить · a авто · ? помощь · q выход"
	var keys string
	switch m.view {
	case viewModules:
		keys = "↑↓ выбор · space вкл/выкл · " + base
	case viewSources:
		keys = "↑↓ выбор · s загрузить · " + base
	case viewIndicators:
		keys = "←→ вид · ↑↓ листать · / поиск · enter действие · " + base
	case viewHits:
		keys = "↑↓ листать · enter действие · " + base
	default:
		keys = base
	}
	var out string
	if m.view == viewIndicators && m.query != "" {
		out = dimStyle.Render(fmt.Sprintf("фильтр «%s» · esc сброс", m.query)) + "\n"
	}
	out += helpStyle.Render(keys)
	if m.err != "" {
		out += "\n" + errStyle.Render("ошибка: "+m.err)
	} else if m.msg != "" {
		out += "\n" + dimStyle.Render(m.msg)
	}
	return out
}

func (m appModel) helpScreen() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("chaff — помощь") + "\n\n")
	keys := [][2]string{
		{"tab / shift+tab", "следующий / предыдущий экран"},
		{"1 … 5", "экран по номеру"},
		{"↑ ↓  (k j)", "двигать выбор / листать"},
		{"r", "обновить"},
		{"a", "авто-обновление вкл/выкл"},
		{"?", "эта справка"},
		{"q", "выход"},
		{"", ""},
		{"space / enter", "Функции: включить / выключить"},
		{"s / enter", "Списки: загрузить источники"},
		{"← →", "Блокировки: сменить вид"},
		{"/", "Блокировки: поиск, esc — сброс"},
		{"enter", "Блокировки/Срабатывания: действие над значением"},
		{"  b / w / d", "заблокировать / разрешить / снять ручное"},
	}
	for _, r := range keys {
		if r[0] == "" {
			b.WriteString("\n")
			continue
		}
		b.WriteString(fmt.Sprintf("  %-18s %s\n", r[0], dimStyle.Render(r[1])))
	}
	b.WriteString("\n" + titleStyle.Render("Функции") + "\n")
	legend := [][2]string{
		{"Врезка в сеть", "прозрачный мост между локалкой и роутером"},
		{"Блокировка по IP", "рвёт соединения к адресам из списка (в ядре)"},
		{"Блокировка по сайтам", "рвёт по имени сайта (SNI / HTTP Host)"},
		{"Анализ DNS", "вычисляет адреса доменов из ответов DNS"},
		{"Обновление списков", "периодически тянет источники"},
	}
	for _, r := range legend {
		b.WriteString(fmt.Sprintf("  %-22s %s\n", r[0], dimStyle.Render(r[1])))
	}
	b.WriteString("\n" + helpStyle.Render("esc/другая — назад · q выход"))
	return b.String()
}

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
