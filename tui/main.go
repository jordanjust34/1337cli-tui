// 1337tui is an interactive front end for 1337cli: search, pick results and a
// qBittorrent category, then it builds the 1337cli command and runs it.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	cli := flag.String("cli", findCLI(), "path to the 1337cli binary (env LEET_CLI)")
	limit := flag.Int("n", 20, "number of results to fetch per search")
	flag.Parse()

	if _, err := exec.LookPath(*cli); err != nil {
		fmt.Fprintf(os.Stderr, "1337tui: can't find 1337cli (%v); install it or pass --cli /path/to/1337cli\n", err)
		os.Exit(1)
	}

	final, err := tea.NewProgram(newModel(*cli, *limit), tea.WithAltScreen()).Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "1337tui:", err)
		os.Exit(1)
	}

	args := final.(model).runArgs
	if args == nil {
		return // quit without running anything
	}

	// The TUI has exited; run the built command in the normal terminal so its
	// progress and any errors stay visible.
	fmt.Println("$", shellJoin(*cli, args))
	cmd := exec.Command(*cli, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			os.Exit(exit.ExitCode())
		}
		fmt.Fprintln(os.Stderr, "1337tui:", err)
		os.Exit(1)
	}
}
