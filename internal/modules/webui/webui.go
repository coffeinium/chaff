package webui

import (
	"context"
	"crypto/tls"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/net/netutil"

	"github.com/coffeinium/chaff/internal/auth"
	"github.com/coffeinium/chaff/internal/config"
	"github.com/coffeinium/chaff/internal/kernel"
)

const maxConns = 256

func init() {
	kernel.Register("webui", func() kernel.Module { return &Module{} })
}

type Module struct {
	k   *kernel.Kernel
	cfg *config.Config
	log *slog.Logger
	am  *auth.Manager
	upd *updater

	useTLS bool
	addr   string

	srv *http.Server
	mu  sync.Mutex
	up  bool
	err error

	gcCancel context.CancelFunc
}

func (m *Module) Name() string  { return "webui" }
func (m *Module) Title() string { return "Веб-интерфейс" }
func (m *Module) About() string {
	return "HTTP-панель управления; вход по токену из CLI"
}
func (m *Module) Needs() []string { return nil }

func (m *Module) Init(k *kernel.Kernel) error {
	m.k = k
	m.cfg = k.Config
	m.log = k.Log
	m.addr = m.cfg.WebAddr
	m.useTLS = wantTLS(m.cfg)
	m.am = auth.NewManager(k.Store, auth.Options{
		Secure:      m.useTLS,
		IdleTTL:     30 * time.Minute,
		AbsoluteTTL: 12 * time.Hour,
	})
	m.upd = &updater{}
	return nil
}

func (m *Module) Start(_ context.Context) error {
	m.srv = &http.Server{
		Handler:           buildHandler(m.k, m.am, m.upd),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       20 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ln, err := net.Listen("tcp", m.addr)
	if err != nil {
		return err
	}
	ln = netutil.LimitListener(ln, maxConns)

	gcCtx, cancel := context.WithCancel(context.Background())
	m.gcCancel = cancel
	go m.gcLoop(gcCtx)
	if m.cfg.WebUpdateCheck {
		go m.updateLoop(gcCtx)
	}

	if m.useTLS {
		cert, err := loadOrGenCert(m.cfg)
		if err != nil {
			ln.Close()
			cancel()
			return err
		}
		m.srv.TLSConfig = &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		}
	} else if !isLoopback(m.addr) {
		m.log.Warn("webui без TLS на нелокальном адресе", "addr", m.addr)
	}

	m.setUp(true, nil)
	go m.serve(ln)
	m.log.Info("webui поднят", "url", m.url(), "tls", m.useTLS)
	return nil
}

func (m *Module) serve(ln net.Listener) {
	var err error
	if m.useTLS {
		err = m.srv.ServeTLS(ln, "", "")
	} else {
		err = m.srv.Serve(ln)
	}
	if err != nil && err != http.ErrServerClosed {
		m.log.Error("webui остановился", "err", err)
		m.setUp(false, err)
	}
}

func (m *Module) Stop(ctx context.Context) error {
	if m.gcCancel != nil {
		m.gcCancel()
	}
	m.setUp(false, nil)
	if m.srv == nil {
		return nil
	}
	return m.srv.Shutdown(ctx)
}

func (m *Module) gcLoop(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.am.GC()
		}
	}
}

func (m *Module) updateLoop(ctx context.Context) {
	m.upd.refresh(ctx)
	t := time.NewTicker(6 * time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.upd.refresh(ctx)
		}
	}
}

func (m *Module) Health() kernel.Health {
	m.mu.Lock()
	up, err := m.up, m.err
	m.mu.Unlock()

	tokens := 0
	if toks, e := m.k.Store.ListTokens(); e == nil {
		tokens = len(toks)
	}
	metrics := map[string]any{
		"url":      m.url(),
		"tls":      m.useTLS,
		"tokens":   tokens,
		"sessions": m.am.Sessions(),
	}
	if err != nil {
		return kernel.Health{OK: false, Detail: err.Error(), Metrics: metrics}
	}
	detail := m.url()
	if tokens == 0 {
		detail += " (нет токенов, chaff web token create)"
	}
	return kernel.Health{OK: up, Detail: detail, Metrics: metrics}
}

func (m *Module) url() string {
	if m.useTLS {
		return "https://" + m.addr
	}
	return "http://" + m.addr
}

func (m *Module) setUp(up bool, err error) {
	m.mu.Lock()
	m.up, m.err = up, err
	m.mu.Unlock()
}

func wantTLS(cfg *config.Config) bool {
	if cfg.WebInsecure {
		return false
	}
	if cfg.WebTLSCert != "" && cfg.WebTLSKey != "" {
		return true
	}
	return !isLoopback(cfg.WebAddr)
}

func isLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
