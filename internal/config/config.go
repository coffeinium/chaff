package config

import (
	"os"
	"path/filepath"
	"strconv"
)

type Config struct {
	DBPath     string
	SocketPath string
	LogLevel   string
	NFQueueNum int

	WebAddr        string
	WebTLSCert     string
	WebTLSKey      string
	WebInsecure    bool
	WebUpdateCheck bool

	DropQUIC bool
}

func Load() *Config {
	return &Config{
		DBPath:         env("CHAFF_DB", "/var/lib/chaff/chaff.db"),
		SocketPath:     env("CHAFF_SOCKET", "/run/chaff.sock"),
		LogLevel:       env("CHAFF_LOG_LEVEL", "info"),
		NFQueueNum:     envInt("CHAFF_NFQUEUE_NUM", 100),
		WebAddr:        env("CHAFF_WEB_ADDR", "0.0.0.0:8787"),
		WebTLSCert:     env("CHAFF_WEB_TLS_CERT", ""),
		WebTLSKey:      env("CHAFF_WEB_TLS_KEY", ""),
		WebInsecure:    os.Getenv("CHAFF_WEB_INSECURE") == "1",
		WebUpdateCheck: os.Getenv("CHAFF_WEB_NO_UPDATE_CHECK") != "1",
		DropQUIC:       os.Getenv("CHAFF_ALLOW_QUIC") != "1",
	}
}

func (c *Config) webDir() string {
	dir := filepath.Dir(c.DBPath)
	if dir == "" || dir == "." {
		dir = "/var/lib/chaff"
	}
	return filepath.Join(dir, "web")
}

func (c *Config) CertFile() string {
	if c.WebTLSCert != "" {
		return c.WebTLSCert
	}
	return filepath.Join(c.webDir(), "cert.pem")
}

func (c *Config) KeyFile() string {
	if c.WebTLSKey != "" {
		return c.WebTLSKey
	}
	return filepath.Join(c.webDir(), "key.pem")
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
