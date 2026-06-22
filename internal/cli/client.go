package cli

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/coffeinium/chaff/internal/config"
	"github.com/coffeinium/chaff/internal/ipc"
)

// cmdClient превращает argv в Request, шлёт демону и печатает ответ.
func cmdClient(argv []string) int {
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
	printData(resp.Data)
	return 0
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

	case "module":
		if len(rest) < 1 {
			return ipc.Request{}, fmt.Errorf("использование: chaff module ls|enable|disable [NAME]")
		}
		args := map[string]string{}
		if len(rest) > 1 {
			args["name"] = rest[1]
		}
		return ipc.Request{Cmd: "module." + rest[0], Args: args}, nil

	case "allow":
		if len(rest) < 1 {
			return ipc.Request{}, fmt.Errorf("использование: chaff allow add|rm|ls [VALUE]")
		}
		args := map[string]string{}
		if len(rest) > 1 {
			args["value"] = rest[1]
		}
		return ipc.Request{Cmd: "allow." + rest[0], Args: args}, nil

	case "source":
		if len(rest) < 1 {
			return ipc.Request{}, fmt.Errorf("использование: chaff source add|ls|sync [...]")
		}
		flags, pos := parseFlags(rest[1:])
		if rest[0] == "sync" && len(pos) > 0 {
			flags["name"] = pos[0]
		}
		return ipc.Request{Cmd: "source." + rest[0], Args: flags}, nil

	case "net":
		if len(rest) < 1 {
			return ipc.Request{}, fmt.Errorf("использование: chaff net up|down|status [--in IF --out IF]")
		}
		flags, _ := parseFlags(rest[1:])
		return ipc.Request{Cmd: "net." + rest[0], Args: flags}, nil

	default:
		return ipc.Request{}, fmt.Errorf("неизвестная команда %q (см. `chaff help`)", group)
	}
}

// parseFlags понимает --key value, --key=value и голые позиционные аргументы.
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

// parseColumnMap превращает "indicator:0,type:1,threat:2" в {indicator:0,...}.
func parseColumnMap(s string) map[string]int {
	out := map[string]int{}
	if s == "" {
		return out
	}
	for _, pair := range strings.Split(s, ",") {
		kv := strings.SplitN(strings.TrimSpace(pair), ":", 2)
		if len(kv) != 2 {
			continue
		}
		if n, err := strconv.Atoi(strings.TrimSpace(kv[1])); err == nil {
			out[strings.TrimSpace(kv[0])] = n
		}
	}
	return out
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
