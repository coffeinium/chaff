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
		fmt.Println(s)
		return
	}
	switch group {
	case "hits":
		renderHits(rows(data))
	case "flows":
		renderFlows(rows(data))
	case "list", "allow", "block":
		renderIndicators(rows(data))
	case "module":
		renderModules(rows(data))
	case "source":
		renderSources(rows(data))
	case "status":
		renderStatus(asMap(data))
	case "test":
		renderTest(asMap(data))
	case "web":
		renderWeb(data)
	default:
		if data == nil {
			return
		}
		printData(data)
	}
}

func renderHits(hs []map[string]any) {
	if len(hs) == 0 {
		fmt.Println(rDim.Render("срабатываний нет"))
		return
	}
	var out [][]string
	for _, h := range hs {
		out = append(out, []string{
			ts(h["ts"]),
			str(h["layer"]),
			str(h["indicator"]),
			str(h["src_ip"]),
			action(str(h["detail"])),
		})
	}
	table([]string{"время", "слой", "индикатор", "источник", "действие"}, out)
}

func renderFlows(fs []map[string]any) {
	if len(fs) == 0 {
		fmt.Println(rDim.Render("соединений нет (включён ли analyzer, поднят ли мост?)"))
		return
	}
	var out [][]string
	for _, f := range fs {
		out = append(out, []string{
			str(f["src_mac"]),
			str(f["src_ip"]),
			str(f["kind"]),
			str(f["dst"]),
			fmt.Sprintf("%s/%d", str(f["proto"]), intOf(f["port"])),
			fmt.Sprintf("%d", intOf(f["packets"])),
			bytesH(intOf(f["bytes"])),
			ts(f["last"]),
		})
	}
	table([]string{"src mac", "src ip", "вид", "назначение", "proto", "пакетов", "байт", "посл."}, out)
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

func renderIndicators(in []map[string]any) {
	if len(in) == 0 {
		fmt.Println(rDim.Render("(пусто)"))
		return
	}
	var out [][]string
	for _, i := range in {
		out = append(out, []string{
			str(i["value"]),
			str(i["kind"]),
			action(str(i["action"])),
			str(i["threat"]),
			str(i["note"]),
		})
	}
	table([]string{"значение", "вид", "действие", "угроза", "причина"}, out)
}

func renderModules(ms []map[string]any) {
	if len(ms) == 0 {
		fmt.Println(rDim.Render("(пусто)"))
		return
	}
	var out [][]string
	for _, m := range ms {
		out = append(out, []string{
			str(m["name"]),
			str(m["title"]),
			onoff(boolean(m["enabled"])),
			runCell(boolean(m["running"]), m["health"]),
			health(m["health"]),
		})
	}
	table([]string{"имя", "функция", "включена", "работает", "здоровье"}, out)
}

func renderSources(ss []map[string]any) {
	if len(ss) == 0 {
		fmt.Println(rDim.Render("источников нет — chaff source add ..."))
		return
	}
	var out [][]string
	for _, s := range ss {
		out = append(out, []string{
			str(s["name"]),
			str(s["adapter"]),
			onoff(boolean(s["enabled"])),
			str(s["uri"]),
		})
	}
	table([]string{"имя", "адаптер", "включён", "источник"}, out)
}

func renderStatus(m map[string]any) {
	if br := asMap(m["bridge"]); br != nil {
		fmt.Println(rHdr.Render("мост") + "  " + bridgeLine(br))
	}
	if mods := rows(m["modules"]); mods != nil {
		running := 0
		for _, x := range mods {
			if boolean(x["running"]) {
				running++
			}
		}
		fmt.Printf("%s  работает %d из %d\n", rHdr.Render("функции"), running, len(mods))
	}
	inds := asMap(m["indicators"])
	if len(inds) > 0 {
		fmt.Println(rHdr.Render("индикаторы"))
		keys := sortedKeys(inds)
		for _, k := range keys {
			fmt.Printf("  %-8s %d\n", k, intOf(inds[k]))
		}
	}
}

func renderTest(m map[string]any) {
	fmt.Printf("%-9s %s\n", "значение", str(m["value"]))
	fmt.Printf("%-9s %s\n", "вид", str(m["kind"]))
	fmt.Printf("%-9s %s\n", "уровень", str(m["layer"]))
	v := str(m["verdict"])
	vs := rDim
	switch v {
	case "block":
		vs = rOff
	case "monitor":
		vs = rWarn
	case "allow":
		vs = rOK
	}
	fmt.Printf("%-9s %s\n", "вердикт", vs.Render(v))
}

func renderWeb(data any) {
	if m := asMap(data); m != nil {
		if tok := str(m["token"]); tok != "" {
			fmt.Println(rHdr.Render("токен создан — сохрани, больше не покажется:"))
			fmt.Println("  " + rOK.Render(tok))
			if intOf(m["expires_at"]) > 0 {
				fmt.Println(rDim.Render("  истекает: " + ts(m["expires_at"])))
			}
			return
		}
		if fp := str(m["fingerprint"]); fp != "" {
			fmt.Printf("%-12s %s\n", "сертификат", str(m["path"]))
			fmt.Printf("%-12s %s\n", "отпечаток", fp)
			return
		}
	}
	renderTokens(rows(data))
}

func renderTokens(toks []map[string]any) {
	if len(toks) == 0 {
		fmt.Println(rDim.Render("токенов нет — chaff web token create"))
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
}

func tokenExpiry(v any) string {
	if intOf(v) == 0 {
		return "никогда"
	}
	return ts(v)
}

func tokenLast(v any) string {
	if intOf(v) == 0 {
		return "—"
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
	line := func(cells []string) {
		var b strings.Builder
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
	line(hcells)
	for _, r := range rows {
		line(r)
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
		return rOK.Render("вкл")
	}
	return rOff.Render("выкл")
}

func runCell(running bool, health any) string {
	if !running {
		return rDim.Render("нет")
	}
	if m := asMap(health); m != nil {
		if ok, has := m["ok"].(bool); has && !ok {
			return rOff.Render("сбой")
		}
	}
	return rOK.Render("да")
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
