package ipc

type Request struct {
	Cmd  string            `json:"cmd"`
	Args map[string]string `json:"args,omitempty"`
}

func (r Request) Arg(k string) string { return r.Args[k] }

type Response struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	Data  any    `json:"data,omitempty"`
}

func OK(data any) Response    { return Response{OK: true, Data: data} }
func Err(msg string) Response { return Response{OK: false, Error: msg} }
