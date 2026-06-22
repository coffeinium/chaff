package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"os"
)

type Handler func(Request) Response

type Server struct {
	path string
	log  *slog.Logger
	mux  map[string]Handler
	ln   net.Listener
}

func NewServer(path string, log *slog.Logger) *Server {
	return &Server{path: path, log: log, mux: make(map[string]Handler)}
}

// Handle вешает обработчик на команду. На неизвестную команду — общая ошибка.
func (s *Server) Handle(cmd string, h Handler) { s.mux[cmd] = h }

// Listen занимает сокет, предварительно убрав старый.
func (s *Server) Listen() error {
	if err := os.Remove(s.path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	ln, err := net.Listen("unix", s.path)
	if err != nil {
		return err
	}
	s.ln = ln
	return nil
}

// Serve принимает соединения, пока не отменят ctx.
func (s *Server) Serve(ctx context.Context) {
	go func() {
		<-ctx.Done()
		s.ln.Close()
	}()
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			s.log.Error("accept не удался", "err", err)
			continue
		}
		go s.handle(conn)
	}
}

func (s *Server) handle(conn net.Conn) {
	defer conn.Close()
	var req Request
	if err := json.NewDecoder(conn).Decode(&req); err != nil {
		_ = json.NewEncoder(conn).Encode(Err("плохой запрос: " + err.Error()))
		return
	}
	h, ok := s.mux[req.Cmd]
	if !ok {
		_ = json.NewEncoder(conn).Encode(Err("неизвестная команда: " + req.Cmd))
		return
	}
	resp := h(req)
	if err := json.NewEncoder(conn).Encode(resp); err != nil {
		s.log.Error("не смог закодировать ответ", "cmd", req.Cmd, "err", err)
	}
}

func (s *Server) Close() error {
	if s.ln != nil {
		return s.ln.Close()
	}
	return nil
}
