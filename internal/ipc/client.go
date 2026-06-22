package ipc

import (
	"encoding/json"
	"fmt"
	"net"
	"time"
)

// Call дозванивается до сокета демона, шлёт один запрос и возвращает ответ.
func Call(socketPath string, req Request) (Response, error) {
	conn, err := net.DialTimeout("unix", socketPath, 3*time.Second)
	if err != nil {
		return Response{}, fmt.Errorf("не подключиться к демону на %s: %w (запущен ли `chaff serve`?)", socketPath, err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))

	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return Response{}, err
	}
	var resp Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil {
		return Response{}, err
	}
	return resp, nil
}
