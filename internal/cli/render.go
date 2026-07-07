package cli

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var (
	rHdr  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("245"))
	rOK   = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	rOff  = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	rWarn = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	rDim  = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
)

func render(group string, data any, jsonOut bool) {
	if jsonOut {
		printData(data)
		return
	}
	if s, ok := data.(string); ok {
		fmt.Println(rOK.Render("✓") + " " + s)
		return
	}
	switch group {
	case "hits":
		renderHits(rows(data))
	case "flows":
		renderFlows(rows(data))
	case "hosts":
		renderHosts(rows(data))
	case "list", "allow", "block":
		renderIndicators(indTitle(group), rows(data))
	case "module":
		renderModules(rows(data))
	case "source":
		if m := asMap(data); m != nil && m["items"] != nil {
			renderSourceIndicators(m)
			return
		}
		renderSources(rows(data))
	case "status":
		renderStatus(asMap(data))
	case "test":
		renderTest(asMap(data))
	case "web":
		renderWeb(data)
	case "group":
		renderGroup(rows(data))
	default:
		if data == nil {
			return
		}
		printData(data)
	}
}

func section(title string) { fmt.Println(rHdr.Render(title)) }

func footer(n int, noun string) {
	fmt.Println(rDim.Render(strings.TrimRight(fmt.Sprintf("всего: %d %s", n, noun), " ")))
}

func empty(msg string) { fmt.Println(rDim.Render("  " + msg)) }

func indTitle(group string) string {
	switch group {
	case "allow":
		return "allow-список"
	case "block":
		return "block-список"
	}
	return "индикаторы"
}

func renderHits(hs []map[string]any) {
	section("срабатывания")
	if len(hs) == 0 {
		empty("пусто")
		return
	}
	type grp struct {
		layer, ind  string
		first, last int64
		count       int
		ips         map[string]bool
	}
	m := map[string]*grp{}
	var order []string
	for _, hrow := range hs {
		layer, ind := str(hrow["layer"]), str(hrow["indicator"])
		tv := int64(intOf(hrow["ts"]))
		key := layer + "|" + ind
		g := m[key]
		if g == nil {
			g = &grp{layer: layer, ind: ind, first: tv, last: tv, ips: map[string]bool{}}
			m[key] = g
			order = append(order, key)
		}
		g.count++
		if tv < g.first {
			g.first = tv
		}
		if tv > g.last {
			g.last = tv
		}
		if s := str(hrow["src_ip"]); s != "" {
			if hn := str(hrow["src_host"]); hn != "" {
				s = hn + " (" + s + ")"
			}
			g.ips[s] = true
		}
	}
	sort.Slice(order, func(i, j int) bool { return m[order[i]].last > m[order[j]].last })
	var out [][]string
	for _, k := range order {
		g := m[k]
		out = append(out, []string{
			rangeFmt(g.first, g.last),
			g.layer,
			g.ind,
			fmt.Sprintf("×%d", g.count),
			ipsShort(g.ips),
		})
	}
	table([]string{"диапазон", "слой", "индикатор", "раз", "источники"}, out)
	fmt.Println(rDim.Render(fmt.Sprintf("событий: %d, сайтов: %d", len(hs), len(order))))
}

func tsSec(sec int64) string {
	if sec == 0 {
		return ""
	}
	return time.Unix(sec, 0).Format("02.01 15:04:05")
}

func rangeFmt(first, last int64) string {
	if first == 0 || first == last {
		return tsSec(last)
	}
	a, b := tsSec(first), tsSec(last)
	if a[:6] == b[:6] {
		return a + "–" + b[6:]
	}
	return a + " – " + b
}

func ipsShort(set map[string]bool) string {
	arr := make([]string, 0, len(set))
	for ip := range set {
		arr = append(arr, ip)
	}
	sort.Strings(arr)
	if len(arr) <= 6 {
		return strings.Join(arr, ", ")
	}
	return strings.Join(arr[:6], ", ") + fmt.Sprintf(" +%d", len(arr)-6)
}

func renderFlows(fs []map[string]any) {
	section("соединения")
	if len(fs) == 0 {
		empty("пусто (включён ли analyzer, поднят ли мост?)")
		return
	}
	var out [][]string
	for _, f := range fs {
		out = append(out, []string{
			str(f["src_host"]),
			str(f["src_mac"]),
			str(f["src_ip"]),
			str(f["kind"]),
			str(f["dst"]),
			fmt.Sprintf("%s/%d", str(f["proto"]), intOf(f["port"])),
			fmt.Sprintf("%d", intOf(f["packets"])),
			bytesH(intOf(f["bytes"])),
			ts(f["last"]),
			action(str(f["verdict"])),
		})
	}
	table([]string{"хост", "src mac", "src ip", "вид", "назначение", "proto", "пакетов", "байт", "посл.", "вердикт"}, out)
	footer(len(fs), "")
}

func renderHosts(hs []map[string]any) {
	section("имена машин")
	if len(hs) == 0 {
		empty("пусто (namesnoop ещё ничего не выучил)")
		return
	}
	var out [][]string
	for _, e := range hs {
		out = append(out, []string{
			str(e["hostname"]),
			str(e["kind"]),
			str(e["key"]),
			str(e["via"]),
			ts(e["seen_at"]),
		})
	}
	table([]string{"имя", "вид", "адрес", "откуда", "обновлено"}, out)
	footer(len(hs), "")
}

func renderIndicators(title string, in []map[string]any) {
	section(title)
	if len(in) == 0 {
		empty("пусто")
		return
	}
	var out [][]string
	for _, i := range in {
		src := "вручную"
		if intOf(i["source_id"]) != 0 {
			src = "фид"
		}
		out = append(out, []string{
			str(i["value"]),
			str(i["kind"]),
			action(str(i["action"])),
			src,
			str(i["note"]),
		})
	}
	table([]string{"значение", "вид", "действие", "откуда", "причина"}, out)
	footer(len(in), "")
}

func renderSourceIndicators(m map[string]any) {
	items := rows(m["items"])
	section(fmt.Sprintf("содержимое %q", str(m["name"])))
	if len(items) == 0 {
		empty("пусто (chaff source sync)")
		return
	}
	var out [][]string
	for _, i := range items {
		out = append(out, []string{
			str(i["value"]),
			str(i["kind"]),
			action(str(i["action"])),
			str(i["note"]),
		})
	}
	table([]string{"значение", "вид", "действие", "причина"}, out)
	total := intOf(m["total"])
	if len(items) < total {
		fmt.Println(rDim.Render(fmt.Sprintf("показано %d из %d (--limit N)", len(items), total)))
		return
	}
	footer(len(items), "")
}

func renderModules(ms []map[string]any) {
	section("функции")
	if len(ms) == 0 {
		empty("пусто")
		return
	}
	run := 0
	var out [][]string
	for _, m := range ms {
		if boolean(m["running"]) {
			run++
		}
		out = append(out, []string{
			str(m["name"]),
			str(m["title"]),
			onoff(boolean(m["enabled"])),
			runCell(boolean(m["running"]), m["health"]),
			health(m["health"]),
		})
	}
	table([]string{"имя", "функция", "вкл", "работа", "здоровье"}, out)
	fmt.Println(rDim.Render(fmt.Sprintf("работает: %d из %d", run, len(ms))))
}

func renderSources(ss []map[string]any) {
	section("источники")
	if len(ss) == 0 {
		empty("пусто (chaff source add ...)")
		return
	}
	var out [][]string
	for _, s := range ss {
		out = append(out, []string{
			str(s["name"]),
			str(s["adapter"]),
			onoff(boolean(s["enabled"])),
			syncCell(s),
			str(s["uri"]),
		})
	}
	table([]string{"имя", "адаптер", "вкл", "синк", "источник"}, out)
	footer(len(ss), "")
}

func syncCell(s map[string]any) string {
	if intOf(s["last_sync"]) == 0 {
		return rDim.Render("ещё не синкался")
	}
	line := fmt.Sprintf("%s · %d · %s", str(s["last_status"]), intOf(s["last_count"]), ts(s["last_sync"]))
	if strings.HasPrefix(str(s["last_status"]), "ok") {
		return rDim.Render(line)
	}
	return rOff.Render(line)
}

func renderStatus(m map[string]any) {
	section("состояние chaff")

	if br := asMap(m["bridge"]); br != nil {
		fmt.Println("\n" + rHdr.Render("мост"))
		fmt.Println(rDim.Render("└─ ") + bridgeGlyph(br) + " " + bridgeLine(br))
	}

	if mods := rows(m["modules"]); mods != nil {
		run := 0
		for _, x := range mods {
			if boolean(x["running"]) {
				run++
			}
		}
		fmt.Println("\n" + rHdr.Render(fmt.Sprintf("функции · работает %d из %d", run, len(mods))))
		for j, x := range mods {
			conn := "├─"
			if j == len(mods)-1 {
				conn = "└─"
			}
			fmt.Printf("%s %s %s %s\n", rDim.Render(conn), runCell(boolean(x["running"]), x["health"]), padName(str(x["name"]), 12), rDim.Render(str(x["title"])))
		}
	}

	if inds := asMap(m["indicators"]); len(inds) > 0 {
		keys := sortedKeys(inds)
		total := 0
		for _, k := range keys {
			total += intOf(inds[k])
		}
		fmt.Println("\n" + rHdr.Render(fmt.Sprintf("индикаторы · всего %d", total)))
		for j, k := range keys {
			conn := "├─"
			if j == len(keys)-1 {
				conn = "└─"
			}
			fmt.Printf("%s %s %d\n", rDim.Render(conn), padName(k, 8), intOf(inds[k]))
		}
	}
}

func renderTest(m map[string]any) {
	section("проверка")
	row := func(conn, k, v string) {
		fmt.Printf("%s %s %s\n", rDim.Render(conn), padName(k, 9), v)
	}
	row("├─", "значение", str(m["value"]))
	row("├─", "вид", str(m["kind"]))
	row("├─", "уровень", str(m["layer"]))
	v := str(m["verdict"])
	g, vs := "·", rDim
	switch v {
	case "block":
		g, vs = "✗", rOff
	case "monitor":
		g, vs = "!", rWarn
	case "allow":
		g, vs = "✓", rOK
	}
	row("└─", "вердикт", vs.Render(g+" "+v))
}

func renderWeb(data any) {
	if m := asMap(data); m != nil {
		if tok := str(m["token"]); tok != "" {
			section("токен создан")
			empty("сохрани, больше не покажется:")
			fmt.Println("  " + rOK.Render(tok))
			if intOf(m["expires_at"]) > 0 {
				empty("истекает: " + ts(m["expires_at"]))
			}
			return
		}
		if fp := str(m["fingerprint"]); fp != "" {
			section("tls-сертификат")
			fmt.Printf("%s %s %s\n", rDim.Render("├─"), padName("файл", 10), str(m["path"]))
			fmt.Printf("%s %s %s\n", rDim.Render("└─"), padName("отпечаток", 10), fp)
			return
		}
	}
	renderTokens(rows(data))
}

func renderTokens(toks []map[string]any) {
	section("токены")
	if len(toks) == 0 {
		empty("пусто (chaff web token create)")
		return
	}
	var out [][]string
	for _, t := range toks {
		out = append(out, []string{
			fmt.Sprintf("%d", intOf(t["id"])),
			str(t["name"]),
			ts(t["created_at"]),
			tokenExpiry(t["expires_at"]),
			tokenLast(t["last_used"]),
		})
	}
	table([]string{"id", "имя", "создан", "истекает", "исп."}, out)
	footer(len(toks), "")
}

func dangerBanner() {
	fmt.Println(rOff.Render("ОПАСНЫЙ ЭКСПЕРИМЕНТАЛЬНЫЙ ФУНКЦИОНАЛ") +
		rDim.Render(" · ручные блокировки в приоритете, машина — только в одной группе"))
}

func renderGroup(data []map[string]any) {
	// scan-кандидаты: строки без имени группы, но с hostname.
	if len(data) > 0 && data[0]["name"] == nil {
		section("кандидаты из сети")
		dangerBanner()
		var out [][]string
		for _, e := range data {
			grp := str(e["group"])
			if grp == "" {
				grp = rDim.Render("—")
			}
			out = append(out, []string{
				str(e["hostname"]), str(e["kind"]), str(e["key"]), grp,
			})
		}
		table([]string{"имя", "вид", "адрес", "группа"}, out)
		footer(len(data), "")
		return
	}

	section("группы")
	dangerBanner()
	if len(data) == 0 {
		empty("пусто (chaff group add ИМЯ)")
		return
	}
	for _, g := range data {
		state := rOff.Render("выкл")
		if boolean(g["enabled"]) {
			state = rOK.Render("вкл")
		}
		head := fmt.Sprintf("%s  %s  %s", rHdr.Render(str(g["name"])), action(str(g["action"])), state)
		if note := str(g["note"]); note != "" {
			head += rDim.Render("  · " + note)
		}
		fmt.Println("\n" + head)
		members := rows(g["members"])
		if len(members) == 0 {
			empty("нет участников (chaff group add-member " + str(g["name"]) + " MAC|ХОСТ)")
			continue
		}
		var out [][]string
		for _, m := range members {
			macs := ""
			if arr, ok := m["macs"].([]any); ok {
				parts := make([]string, 0, len(arr))
				for _, x := range arr {
					parts = append(parts, str(x))
				}
				macs = strings.Join(parts, ", ")
			}
			resolved := rOK.Render("да")
			if !boolean(m["resolved"]) {
				resolved = rWarn.Render("ждёт")
			}
			out = append(out, []string{
				str(m["value"]), str(m["kind"]), str(m["hostname"]), macs, resolved,
			})
		}
		table([]string{"участник", "вид", "имя", "mac", "готов"}, out)
	}
}

func bytesH(n int) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1fG", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1fM", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1fK", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%dB", n)
}

func tokenExpiry(v any) string {
	if intOf(v) == 0 {
		return "никогда"
	}
	return ts(v)
}

func tokenLast(v any) string {
	if intOf(v) == 0 {
		return "-"
	}
	return ts(v)
}

func bridgeLine(br map[string]any) string {
	detail := str(br["detail"])
	switch {
	case !boolean(br["running"]):
		return rDim.Render("выключен")
	case boolean(br["up"]):
		return rOK.Render(detail)
	case !boolean(br["configured"]):
		return rWarn.Render(detail)
	default:
		return rOff.Render(detail)
	}
}

func bridgeGlyph(br map[string]any) string {
	switch {
	case !boolean(br["running"]):
		return rOff.Render("✗")
	case boolean(br["up"]):
		return rOK.Render("✓")
	default:
		return rWarn.Render("!")
	}
}

func table(headers []string, rows [][]string) {
	w := make([]int, len(headers))
	for i, h := range headers {
		w[i] = lipgloss.Width(h)
	}
	for _, r := range rows {
		for i, c := range r {
			if cw := lipgloss.Width(c); cw > w[i] {
				w[i] = cw
			}
		}
	}
	line := func(prefix string, cells []string) {
		var b strings.Builder
		b.WriteString(prefix)
		for i, c := range cells {
			b.WriteString(pad(c, w[i]))
			if i < len(cells)-1 {
				b.WriteString("  ")
			}
		}
		fmt.Println(strings.TrimRight(b.String(), " "))
	}
	hcells := make([]string, len(headers))
	for i, h := range headers {
		hcells[i] = rHdr.Render(h)
	}
	line(rDim.Render("  "), hcells)
	for _, r := range rows {
		line("  ", r)
	}
}

func pad(s string, w int) string {
	d := w - lipgloss.Width(s)
	if d <= 0 {
		return s
	}
	return s + strings.Repeat(" ", d)
}

func rows(data any) []map[string]any {
	arr, ok := data.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(arr))
	for _, x := range arr {
		if m, ok := x.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func asMap(data any) map[string]any {
	m, _ := data.(map[string]any)
	return m
}

func str(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprintf("%v", v)
}

func boolean(v any) bool {
	b, _ := v.(bool)
	return b
}

func intOf(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	}
	return 0
}

func ts(v any) string {
	sec := int64(intOf(v))
	if sec == 0 {
		return ""
	}
	return time.Unix(sec, 0).Format("02.01 15:04:05")
}

func action(a string) string {
	switch a {
	case "block":
		return rOff.Render(a)
	case "monitor":
		return rWarn.Render(a)
	case "allow":
		return rOK.Render(a)
	}
	return a
}

func onoff(b bool) string {
	if b {
		return rOK.Render("✓")
	}
	return rOff.Render("✗")
}

func runCell(running bool, health any) string {
	if !running {
		return rOff.Render("✗")
	}
	if m := asMap(health); m != nil {
		if ok, has := m["ok"].(bool); has && !ok {
			return rWarn.Render("!")
		}
	}
	return rOK.Render("✓")
}

func health(v any) string {
	m := asMap(v)
	if m == nil {
		return ""
	}
	d := str(m["detail"])
	if boolean(m["ok"]) {
		return rDim.Render(d)
	}
	return rOff.Render(d)
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
