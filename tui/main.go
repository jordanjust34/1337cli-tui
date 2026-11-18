// 1337tui is an interactive front end for searching 1337x and sending torrents
// to qBittorrent: search, pick results and a category, confirm. It shares the
// internal/core library with 1337cli and works without it installed.
package main

import (
	"flag"
	"fmt"
	"os"

	"1337cli/internal/core"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	core.LoadConfig()
	limit := flag.Int("n", 20, "number of results to fetch per search")
	flag.Parse()

	cfg := core.FromEnv()
	defaults := core.AddOptions{Category: cfg.Category, SavePath: cfg.SavePath}
	final, err := tea.NewProgram(newModel(core.New(cfg), defaults, *limit), tea.WithAltScreen()).Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "1337tui:", err)
		os.Exit(1)
	}
	if final.(model).failed {
		os.Exit(1)
	}
}
