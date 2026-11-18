package core

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// Settings can come from flags (CLI only), environment variables, or a config
// file, in that order of precedence. The file holds the same KEY=value pairs as
// the env vars (QBIT_URL, FLARESOLVERR_URL, ...) so a server only needs it set
// up once, and both binaries read the same file.

// DefaultMirror is the 1337x mirror used when LEET_MIRROR isn't set.
const DefaultMirror = "https://1337x.to"

// Config is everything the shared operations need to reach 1337x and qBittorrent.
type Config struct {
	Mirror       string // 1337x base URL
	FlareSolverr string // FlareSolverr endpoint; empty fetches pages directly
	Cookie       string // raw Cookie header for direct fetches
	UserAgent    string // empty keeps the scraper's default

	QbitURL  string
	QbitUser string // empty skips login
	QbitPass string

	// Submission defaults: the category and save path used when the caller
	// doesn't choose one (QBIT_CATEGORY, QBIT_SAVE_PATH).
	Category string
	SavePath string
}

// FromEnv builds a Config from the environment. Call LoadConfig first so the
// config file's values are in the environment.
func FromEnv() Config {
	mirror := os.Getenv("LEET_MIRROR")
	if mirror == "" {
		mirror = DefaultMirror
	}
	return Config{
		Mirror:       mirror,
		FlareSolverr: os.Getenv("FLARESOLVERR_URL"),
		Cookie:       os.Getenv("LEET_COOKIE"),
		UserAgent:    os.Getenv("LEET_USER_AGENT"),
		QbitURL:      os.Getenv("QBIT_URL"),
		QbitUser:     os.Getenv("QBIT_USER"),
		QbitPass:     os.Getenv("QBIT_PASS"),
		Category:     os.Getenv("QBIT_CATEGORY"),
		SavePath:     os.Getenv("QBIT_SAVE_PATH"),
	}
}

// ConfigPath is ~/.config/1337cli/config on Linux, overridable with LEET_CONFIG.
func ConfigPath() string {
	if p := os.Getenv("LEET_CONFIG"); p != "" {
		return p
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "1337cli", "config")
}

// LoadConfig copies KEY=value lines from the config file into the environment,
// without overriding variables that are already set. It returns the path read.
func LoadConfig() string {
	path := ConfigPath()
	f, err := os.Open(path)
	if err != nil {
		return path
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, val, ok := strings.Cut(strings.TrimPrefix(line, "export "), "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.Trim(strings.TrimSpace(val), `"'`)
		if _, set := os.LookupEnv(key); !set {
			os.Setenv(key, val)
		}
	}
	return path
}
