package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime configuration, sourced from environment variables.
type Config struct {
	Addr          string
	DataDir       string
	DBPath        string
	AdminUser     string
	AdminPassword string
	Secret        string
	CookieSecure  bool
	SessionTTL    time.Duration
	IMAPHost      string
	IMAPPort      int
	// PlaintextIMAP disables TLS for IMAP. Only for local tests.
	PlaintextIMAP    bool
	FetchConcurrency int
	FetchTimeout     time.Duration
	FetchLimit       int
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v, ok := os.LookupEnv(key); ok {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}
	return def
}

func envBool(key string, def bool) bool {
	if v, ok := os.LookupEnv(key); ok {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	return def
}

// Load reads configuration from the environment.
func Load() Config {
	dataDir := env("GM_DATA_DIR", "/data")
	return Config{
		Addr:             env("GM_ADDR", ":8080"),
		DataDir:          dataDir,
		DBPath:           env("GM_DB_PATH", filepath.Join(dataDir, "gmailmanager.db")),
		AdminUser:        env("GM_ADMIN_USER", "admin"),
		AdminPassword:    env("GM_ADMIN_PASSWORD", "admin123"),
		Secret:           env("GM_SECRET", ""),
		CookieSecure:     envBool("GM_COOKIE_SECURE", false),
		SessionTTL:       time.Duration(envInt("GM_SESSION_HOURS", 12)) * time.Hour,
		IMAPHost:         env("GM_IMAP_HOST", "imap.gmail.com"),
		IMAPPort:         envInt("GM_IMAP_PORT", 993),
		PlaintextIMAP:    envBool("GM_IMAP_PLAINTEXT", false),
		FetchConcurrency: envInt("GM_FETCH_CONCURRENCY", 6),
		FetchTimeout:     time.Duration(envInt("GM_FETCH_TIMEOUT_SECONDS", 60)) * time.Second,
		FetchLimit:       envInt("GM_FETCH_LIMIT", 10),
	}
}

// IsDefaultAdminPassword reports whether the built-in admin password is still in use.
func (c Config) IsDefaultAdminPassword() bool {
	return c.AdminPassword == "admin123"
}
