package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/coffeinium/chaff/internal/config"
	"github.com/coffeinium/chaff/internal/ipc"
)

func cmdClient(argv []string) int {
	jsonOut := false
	filtered := argv[:0:0]
	for _, a := range argv {
		if a == "--json" {
			jsonOut = true
			continue
		}
		filtered = append(filtered, a)
	}
	argv = filtered
	if len(argv) == 0 {
		return errln("нет команды")
	}

	req, err := buildRequest(argv)
	if err != nil {
		return errln("%v", err)
	}
	resp, err := ipc.Call(config.Load().SocketPath, req)
	if err != nil {
		return errln("%v", err)
	}
	if !resp.OK {
		return errln("%s", resp.Error)
	}
	render(argv[0], resp.Data, jsonOut)
	return statusExit(argv[0], resp.Data)
}

func statusExit(group string, data any) int {
	if group != "status" {
		return 0
	}
	m, ok := data.(map[string]any)
	if !ok {
		return 1
	}
	br, _ := m["bridge"].(map[string]any)
	if up, _ := br["up"].(bool); up {
		return 0
	}
	return 1
}

func buildRequest(argv []string) (ipc.Request, error) {
	group := argv[0]
	rest := argv[1:]
	switch group {
	case "status", "apply":
		return ipc.Request{Cmd: group}, nil

	case "list":
		if len(rest) < 1 {
			return ipc.Request{}, fmt.Errorf("использование: chaff list ip|cidr|domain|url|sha256|md5")
		}
		return ipc.Request{Cmd: "list", Args: map[string]string{"kind": rest[0]}}, nil

	case "test":
		if len(rest) < 1 {
			return ipc.Request{}, fmt.Errorf("использование: chaff test VALUE")
		}
		return ipc.Request{Cmd: "test", Args: map[string]string{"value": rest[0]}}, nil

	case "hits":
		args := map[string]string{}
		if len(rest) > 0 {
			args["limit"] = rest[0]
		}
		return ipc.Request{Cmd: "hits", Args: args}, nil

	case "flows":
		args := map[string]string{}
		if len(rest) > 0 {
			args["limit"] = rest[0]
		}
		return ipc.Request{Cmd: "analyzer.flows", Args: args}, nil

	case "module":
		if len(rest) < 1 {
			return ipc.Request{}, fmt.Errorf("использование: chaff module ls|enable|disable [NAME]")
		}
		args := map[string]string{}
		if len(rest) > 1 {
			args["name"] = rest[1]
		}
		return ipc.Request{Cmd: "module." + rest[0], Args: args}, nil

	case "allow", "block":
		if len(rest) < 1 {
			return ipc.Request{}, fmt.Errorf("использование: chaff %s add|rm [VALUE] [--note ПРИЧИНА]", group)
		}
		flags, pos := parseFlags(rest[1:])
		if len(pos) > 0 {
			flags["value"] = pos[0]
		}
		return ipc.Request{Cmd: group + "." + rest[0], Args: flags}, nil

	case "source":
		if len(rest) < 1 {
			return ipc.Request{}, fmt.Errorf("использование: chaff source add|ls|sync|enable|disable [...]")
		}
		flags, pos := parseFlags(rest[1:])
		switch rest[0] {
		case "sync", "enable", "disable":
			if len(pos) > 0 {
				flags["name"] = pos[0]
			}
		}
		return ipc.Request{Cmd: "source." + rest[0], Args: flags}, nil

	case "net":
		if len(rest) < 1 {
			return ipc.Request{}, fmt.Errorf("использование: chaff net up|down|status [--in IF --out IF]")
		}
		flags, _ := parseFlags(rest[1:])
		return ipc.Request{Cmd: "net." + rest[0], Args: flags}, nil

	case "web":
		if len(rest) < 1 {
			return ipc.Request{}, fmt.Errorf("использование: chaff web token create|ls|rm [...] | web cert")
		}
		switch rest[0] {
		case "token":
			if len(rest) < 2 {
				return ipc.Request{}, fmt.Errorf("использование: chaff web token create|ls|rm [...]")
			}
			flags, pos := parseFlags(rest[2:])
			if rest[1] == "rm" && len(pos) > 0 {
				flags["ref"] = pos[0]
			}
			return ipc.Request{Cmd: "web.token." + rest[1], Args: flags}, nil
		case "cert":
			return ipc.Request{Cmd: "web.cert"}, nil
		default:
			return ipc.Request{}, fmt.Errorf("использование: chaff web token ... | web cert")
		}

	default:
		return ipc.Request{}, fmt.Errorf("неизвестная команда %q (см. `chaff help`)", group)
	}
}

func parseFlags(args []string) (map[string]string, []string) {
	flags := map[string]string{}
	var pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "--") {
			k := strings.TrimPrefix(a, "--")
			if eq := strings.IndexByte(k, '='); eq >= 0 {
				flags[k[:eq]] = k[eq+1:]
				continue
			}
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				flags[k] = args[i+1]
				i++
			} else {
				flags[k] = "true"
			}
			continue
		}
		pos = append(pos, a)
	}
	return flags, pos
}

func printData(data any) {
	switch v := data.(type) {
	case nil:
		return
	case string:
		fmt.Println(v)
	default:
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			fmt.Printf("%v\n", v)
			return
		}
		fmt.Println(string(b))
	}
}
