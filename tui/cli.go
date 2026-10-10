package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// result mirrors the JSON emitted by `1337cli -s ... --json`.
type result struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Seeders  int    `json:"seeders"`
	Leechers int    `json:"leechers"`
	Date     string `json:"date"`
	Size     string `json:"size"`
	Uploader string `json:"uploader"`
}

// category mirrors the JSON emitted by `1337cli categories --json`.
type category struct {
	Name     string `json:"name"`
	SavePath string `json:"savePath"`
}

// findCLI locates the 1337cli binary: $LEET_CLI, then next to this program,
// then $PATH.
func findCLI() string {
	if p := os.Getenv("LEET_CLI"); p != "" {
		return p
	}
	if self, err := os.Executable(); err == nil {
		name := "1337cli"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		sibling := filepath.Join(filepath.Dir(self), name)
		if _, err := os.Stat(sibling); err == nil {
			return sibling
		}
	}
	if p, err := exec.LookPath("1337cli"); err == nil {
		return p
	}
	return "1337cli"
}

// runJSON runs the CLI and decodes its stdout as JSON into v. On failure the
// error carries the CLI's stderr, which explains what went wrong.
func runJSON(ctx context.Context, cli string, args []string, v any) error {
	var stdout, stderr bytes.Buffer
	c := exec.CommandContext(ctx, cli, args...)
	c.Stdout, c.Stderr = &stdout, &stderr
	if err := c.Run(); err != nil {
		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return errors.New(msg)
		}
		return err
	}
	return json.Unmarshal(stdout.Bytes(), v)
}

func searchArgs(query string, limit int) []string {
	// --search=... keeps a query starting with "-" from being read as a flag.
	return []string{"--search=" + query, "--limit", strconv.Itoa(limit), "--json"}
}

// getArgs builds the CLI arguments that send the chosen torrents to qBittorrent.
func getArgs(selected []result, category string, paused bool) []string {
	args := []string{"get"}
	for _, r := range selected {
		args = append(args, r.URL)
	}
	if category != "" {
		args = append(args, "--category", category)
	}
	if paused {
		args = append(args, "--paused")
	}
	return args
}

// shellJoin renders a command line that can be pasted into a POSIX shell.
func shellJoin(name string, args []string) string {
	parts := []string{shellQuote(name)}
	for _, a := range args {
		parts = append(parts, shellQuote(a))
	}
	return strings.Join(parts, " ")
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	safe := true
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_./:=,@%+", r)) {
			safe = false
			break
		}
	}
	if safe {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
