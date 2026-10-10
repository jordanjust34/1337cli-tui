package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestShellQuote(t *testing.T) {
	cases := map[string]string{
		"https://1337x.to/torrent/1/Ubuntu-16-04/": "https://1337x.to/torrent/1/Ubuntu-16-04/",
		"movies":      "movies",
		"tv shows":    "'tv shows'",
		"it's":        `'it'\''s'`,
		"":            "''",
		"$(rm -rf ~)": "'$(rm -rf ~)'",
		"--category":  "--category",
		"a;b":         "'a;b'",
	}
	for in, want := range cases {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %s, want %s", in, got, want)
		}
	}
}

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case " ":
		return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func send(m model, msgs ...tea.Msg) model {
	for _, msg := range msgs {
		next, _ := m.Update(msg)
		m = next.(model)
	}
	return m
}

func TestFlowBuildsCommand(t *testing.T) {
	results := []result{
		{Name: "Superman (2025)", URL: "https://1337x.to/torrent/1/a/"},
		{Name: "Superman Returns", URL: "https://1337x.to/torrent/2/b/"},
		{Name: "Superman (1978)", URL: "https://1337x.to/torrent/3/c/"},
	}
	m := newModel("1337cli", 20)
	m = send(m, categoriesMsg{cats: []category{{Name: "movies", SavePath: "/media/movies"}, {Name: "tv", SavePath: "/media/tv"}}})

	m = send(m, key("superman"), key("enter"))
	if m.screen != screenSearching || m.query != "superman" {
		t.Fatalf("after enter: screen %v query %q", m.screen, m.query)
	}

	// A result from an older search must be ignored.
	m = send(m, searchDoneMsg{seq: m.seq - 1, results: results[:1]})
	if m.screen != screenSearching {
		t.Fatal("stale search result was accepted")
	}
	m = send(m, searchDoneMsg{seq: m.seq, results: results})
	if m.screen != screenResults {
		t.Fatalf("screen = %v, want results", m.screen)
	}

	// Select #1 and #3 (space advances the cursor).
	m = send(m, key(" "), key("down"), key(" "))
	m = send(m, key("enter")) // to category
	m = send(m, key("down"))  // "movies"
	m = send(m, key("enter")) // to confirm
	if m.screen != screenConfirm {
		t.Fatalf("screen = %v, want confirm", m.screen)
	}
	m = send(m, key("enter"))

	want := []string{"get", "https://1337x.to/torrent/1/a/", "https://1337x.to/torrent/3/c/", "--category", "movies"}
	if !reflect.DeepEqual(m.runArgs, want) {
		t.Fatalf("runArgs = %q\nwant      %q", m.runArgs, want)
	}
}

func TestNothingSelectedUsesCursorAndQuitRunsNothing(t *testing.T) {
	m := newModel("1337cli", 20)
	m = send(m, key("x"), key("enter"))
	m = send(m, searchDoneMsg{seq: m.seq, results: []result{{URL: "u1"}, {URL: "u2"}}})
	m = send(m, key("down"), key("enter"), key("enter"), key("p"))
	if got := m.commandArgs(); !reflect.DeepEqual(got, []string{"get", "u2", "--paused"}) {
		t.Fatalf("commandArgs = %q", got)
	}
	m = send(m, key("q"))
	if m.runArgs != nil {
		t.Fatalf("quit should not run anything, got %q", m.runArgs)
	}
}

// TestRunJSONWithFakeCLI checks the TUI's contract with the real binary: the
// arguments it passes and the JSON / error handling.
func TestRunJSONWithFakeCLI(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script")
	}
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	cli := filepath.Join(dir, "1337cli")
	script := `#!/bin/sh
printf '%s\n' "$@" > ` + argsFile + `
case "$1" in
  --search=fail) echo "blocked by Cloudflare challenge" >&2; exit 1 ;;
esac
echo '[{"name":"Movie","url":"https://1337x.to/torrent/9/m/","seeders":5,"size":"1 GB"}]'
`
	if err := os.WriteFile(cli, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	var rs []result
	if err := runJSON(context.Background(), cli, searchArgs("-dash query", 30), &rs); err != nil {
		t.Fatal(err)
	}
	if len(rs) != 1 || rs[0].Seeders != 5 || rs[0].URL != "https://1337x.to/torrent/9/m/" {
		t.Fatalf("results = %+v", rs)
	}
	got, _ := os.ReadFile(argsFile)
	if want := "--search=-dash query\n--limit\n30\n--json\n"; string(got) != want {
		t.Fatalf("CLI args = %q, want %q", got, want)
	}

	err := runJSON(context.Background(), cli, searchArgs("fail", 20), &rs)
	if err == nil || !strings.Contains(err.Error(), "Cloudflare") {
		t.Fatalf("err = %v, want the CLI's stderr message", err)
	}
}
