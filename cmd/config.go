package cmd

import (
	"fmt"
	"os"

	"1337cli/internal/core"
)

// Loaded from a package-level var so it runs before any init() reads os.Getenv
// for flag defaults. See core.LoadConfig for the file format and precedence.
var configPath = core.LoadConfig()

// newService builds the shared service from the parsed flags, which already
// default to the environment and config file.
func newService() *core.Service {
	s := core.New(core.Config{
		Mirror:       mirror,
		FlareSolverr: flareURL,
		Cookie:       cookie,
		UserAgent:    userAgent,
		QbitURL:      qbitURL,
		QbitUser:     qbitUser,
		QbitPass:     qbitPass,
		Category:     qbitCategory,
		SavePath:     qbitSavePath,
	})
	s.Warn = func(msg string) { fmt.Fprintln(os.Stderr, "warning: "+msg) }
	return s
}
