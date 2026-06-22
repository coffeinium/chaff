package kernel

import (
	"fmt"
	"sort"
)

// topoSort упорядочивает модули так, чтобы каждый стартовал после тех, что ему
// нужны (Needs). Ошибка — на отсутствующую зависимость или цикл.
func topoSort(inst map[string]Module) ([]string, error) {
	indeg := make(map[string]int, len(inst))
	deps := make(map[string][]string, len(inst)) // зависимость -> зависящие
	for name := range inst {
		if _, ok := indeg[name]; !ok {
			indeg[name] = 0
		}
		for _, dep := range inst[name].Needs() {
			if _, ok := inst[dep]; !ok {
				return nil, fmt.Errorf("модулю %q нужен %q, которого нет", name, dep)
			}
			deps[dep] = append(deps[dep], name)
			indeg[name]++
		}
	}

	// Алгоритм Кана; имена берём в отсортированном порядке для стабильного вывода.
	var ready []string
	for name, d := range indeg {
		if d == 0 {
			ready = append(ready, name)
		}
	}
	sort.Strings(ready)

	var order []string
	for len(ready) > 0 {
		n := ready[0]
		ready = ready[1:]
		order = append(order, n)
		var freed []string
		for _, dependent := range deps[n] {
			indeg[dependent]--
			if indeg[dependent] == 0 {
				freed = append(freed, dependent)
			}
		}
		sort.Strings(freed)
		ready = append(ready, freed...)
	}

	if len(order) != len(inst) {
		return nil, fmt.Errorf("цикл в зависимостях модулей")
	}
	return order, nil
}
