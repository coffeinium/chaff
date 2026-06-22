// Пакет ipc — канал управления между тонким CLI-клиентом и демоном. В SQLite
// пишет только демон; клиент шлёт команды сюда.
package ipc

// Request — одна команда CLI. Cmd с точкой (например "module.enable"); Args —
// нарочно простая плоская map.
type Request struct {
	Cmd  string            `json:"cmd"`
	Args map[string]string `json:"args,omitempty"`
}

func (r Request) Arg(k string) string { return r.Args[k] }

// Response — ответ демона. Data — произвольный JSON.
type Response struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	Data  any    `json:"data,omitempty"`
}

func OK(data any) Response    { return Response{OK: true, Data: data} }
func Err(msg string) Response { return Response{OK: false, Error: msg} }
