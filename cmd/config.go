package cmd

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// Settings can come from flags, environment variables, or a config file, in
// that order of precedence. The file holds the same KEY=value pairs as the
// env vars (QBIT_URL, FLARESOLVERR_URL, ...) so a server only needs it set up once.
//
// Loaded from a package-level var so it runs before any init() reads os.Getenv
// for flag defaults.
var configPath = loadConfig()

// defaultConfigPath is ~/.config/1337cli/config on Linux, overridable with LEET_CONFIG.
func defaultConfigPath() string {
	if p := os.Getenv("LEET_CONFIG"); p != "" {
		return p
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "1337cli", "config")
}

// loadConfig copies KEY=value lines from the config file into the environment,
// without overriding variables that are already set. It returns the path read.
func loadConfig() string {
	path := defaultConfigPath()
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
