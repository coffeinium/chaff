package config

import (
	"os"
	"strconv"
)

type Config struct {
	DBPath     string
	SocketPath string
	LogLevel   string
	NFQueueNum int
}

func Load() *Config {
	return &Config{
		DBPath:     env("CHAFF_DB", "/var/lib/chaff/chaff.db"),
		SocketPath: env("CHAFF_SOCKET", "/run/chaff.sock"),
		LogLevel:   env("CHAFF_LOG_LEVEL", "info"),
		NFQueueNum: envInt("CHAFF_NFQUEUE_NUM", 100),
	}
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
