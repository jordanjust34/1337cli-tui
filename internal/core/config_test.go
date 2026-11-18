package core

import (
	"os"
	"path/filepath"
	"testing"
)

// unsetenv clears key for this test and restores it afterwards, so values
// LoadConfig sets don't leak into other tests.
func unsetenv(t *testing.T, key string) {
	t.Setenv(key, "")
	os.Unsetenv(key)
}

func TestLoadConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	os.WriteFile(path, []byte(`# comment
QBIT_URL=http://file:8080
export QBIT_USER="admin"
QBIT_PASS='s3cret'
  FLARESOLVERR_URL = http://localhost:8191
not a setting
QBIT_CATEGORY=
`), 0o600)
	t.Setenv("LEET_CONFIG", path)
	for _, k := range []string{"QBIT_URL", "QBIT_USER", "QBIT_PASS", "FLARESOLVERR_URL", "QBIT_CATEGORY", "LEET_MIRROR"} {
		unsetenv(t, k)
	}
	t.Setenv("QBIT_PASS", "from-env") // the environment beats the file

	if got := LoadConfig(); got != path {
		t.Errorf("LoadConfig() = %q, want %q", got, path)
	}
	cfg := FromEnv()
	want := Config{
		Mirror:       DefaultMirror,
		FlareSolverr: "http://localhost:8191",
		QbitURL:      "http://file:8080",
		QbitUser:     "admin",
		QbitPass:     "from-env",
	}
	if cfg != want {
		t.Errorf("FromEnv() = %+v\nwant        %+v", cfg, want)
	}
}

func TestLoadConfigMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nope")
	t.Setenv("LEET_CONFIG", path)
	if got := LoadConfig(); got != path {
		t.Errorf("LoadConfig() = %q, want %q", got, path)
	}
}
