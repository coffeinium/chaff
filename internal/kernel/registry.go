package kernel

import (
	"fmt"
	"sort"
)

type Factory func() Module

var registry = make(map[string]Factory)

func Register(name string, f Factory) {
	if _, dup := registry[name]; dup {
		panic("kernel: повторная регистрация модуля: " + name)
	}
	registry[name] = f
}

func Registered() []string {
	names := make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func instantiate(name string) (Module, error) {
	f, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("неизвестный модуль %q", name)
	}
	return f(), nil
}

func Describe(name string) (title, about string) {
	m, err := instantiate(name)
	if err != nil {
		return name, ""
	}
	if d, ok := m.(Describer); ok {
		return d.Title(), d.About()
	}
	return name, ""
}
