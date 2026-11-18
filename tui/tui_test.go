package main

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"1337cli/internal/core"

	tea "github.com/charmbracelet/bubbletea"
)

// recorder is a fake backend that records submissions.
type recorder struct {
	submits []submission
	failAdd string // target whose submission fails
	pingErr error
}

type submission struct {
	targets []string
	opts    core.AddOptions
}

func (r *recorder) Search(string, int, int) ([]core.Result, error) { return nil, nil }

func (r *recorder) Categories() ([]core.Category, error) { return nil, nil }

func (r *recorder) Ping() (string, error) {
	if r.pingErr != nil {
		return "", r.pingErr
	}
	return "v5.0.4", nil
}

func (r *recorder) Submit(targets []string, opts core.AddOptions, _ func(core.Outcome)) ([]core.Outcome, error) {
	r.submits = append(r.submits, submission{targets, opts})
	var outs []core.Outcome
	for _, t := range targets {
		o := core.Outcome{Target: t, Name: t}
		if t == r.failAdd {
			o.Err = errors.New("rejected")
		}
		outs = append(outs, o)
	}
	return outs, nil
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

// send feeds messages to the model, discarding the commands it returns.
func send(m model, msgs ...tea.Msg) model {
	for _, msg := range msgs {
		next, _ := m.Update(msg)
		m = next.(model)
	}
	return m
}

// confirm presses enter on the confirm screen and runs the resulting submit
// command, as Bubble Tea would, feeding its result back to the model.
func confirm(t *testing.T, m model) model {
	t.Helper()
	next, cmd := m.Update(key("enter"))
	m = next.(model)
	if m.screen != screenSubmitting {
		t.Fatalf("screen = %v, want submitting", m.screen)
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatal("confirm should return a batch of spinner tick and submit")
	}
	for _, c := range batch {
		if msg, ok := c().(submitDoneMsg); ok {
			return send(m, msg)
		}
	}
	t.Fatal("no submit command in batch")
	return m
}

var results = []core.Result{
	{Name: "Superman (2025)", URL: "https://1337x.to/torrent/1/a/"},
	{Name: "Superman Returns", URL: "https://1337x.to/torrent/2/b/"},
	{Name: "Superman (1978)", URL: "https://1337x.to/torrent/3/c/"},
}

// toConfirm searches, selects #1 and #3, picks "movies", and stops at the confirm screen.
func toConfirm(t *testing.T, rec *recorder) model {
	t.Helper()
	m := newModel(rec, core.AddOptions{}, 20)
	m = send(m, categoriesMsg{cats: []core.Category{{Name: "movies", SavePath: "/media/movies"}, {Name: "tv", SavePath: "/media/tv"}}})

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
	return m
}

func TestConfirmSubmitsOneBatch(t *testing.T) {
	rec := &recorder{}
	m := confirm(t, toConfirm(t, rec))

	want := []submission{{
		targets: []string{"https://1337x.to/torrent/1/a/", "https://1337x.to/torrent/3/c/"},
		opts:    core.AddOptions{Category: "movies"},
	}}
	if !reflect.DeepEqual(rec.submits, want) {
		t.Fatalf("submits = %+v\nwant      %+v", rec.submits, want)
	}
	if m.screen != screenDone || m.failed {
		t.Fatalf("screen = %v failed = %v, want done without failure", m.screen, m.failed)
	}
}

func TestQuitAtConfirmSendsNothing(t *testing.T) {
	rec := &recorder{}
	send(toConfirm(t, rec), key("q"))
	send(toConfirm(t, rec), key("esc"), key("esc"), key("q"))
	if len(rec.submits) != 0 {
		t.Fatalf("leaving confirm must not submit, got %+v", rec.submits)
	}
}

func TestNothingSelectedUsesCursorAndDefaults(t *testing.T) {
	rec := &recorder{failAdd: "u2"}
	m := newModel(rec, core.AddOptions{Category: "linux", SavePath: "/dl"}, 20)
	m = send(m, key("x"), key("enter"))
	m = send(m, searchDoneMsg{seq: m.seq, results: []core.Result{{URL: "u1"}, {URL: "u2"}}})
	m = send(m, key("down"), key("enter"), key("enter"), key("p"))
	m = confirm(t, m)

	want := []submission{{targets: []string{"u2"}, opts: core.AddOptions{Category: "linux", SavePath: "/dl", Paused: true}}}
	if !reflect.DeepEqual(rec.submits, want) {
		t.Fatalf("submits = %+v\nwant      %+v", rec.submits, want)
	}
	if !m.failed {
		t.Fatal("a failed submission should make the TUI exit non-zero")
	}
}

func TestBestSelectsTopSeeded(t *testing.T) {
	rec := &recorder{}
	m := newModel(rec, core.AddOptions{}, 20)
	m = send(m, key("x"), key("enter"))
	m = send(m, searchDoneMsg{seq: m.seq, results: []core.Result{
		{URL: "u1", Seeders: 5},
		{URL: "u2", Seeders: 40},
		{URL: "u3", Seeders: 12},
		{URL: "u4", Seeders: 40}, // tie: the earlier row wins
	}})
	// An earlier manual selection is replaced, not added to.
	m = send(m, key(" "), key("b"))
	if m.cursor != 1 || !reflect.DeepEqual(m.chosenExplicit(), []int{1}) {
		t.Fatalf("cursor = %d selected = %v, want only row 1", m.cursor, m.chosenExplicit())
	}
	m = send(m, key("enter"), key("enter")) // category (none), confirm
	confirm(t, m)
	if len(rec.submits) != 1 || !reflect.DeepEqual(rec.submits[0].targets, []string{"u2"}) {
		t.Fatalf("submits = %+v, want just u2", rec.submits)
	}
}

// startup runs the model's Init commands and feeds back the ping result.
func startup(t *testing.T, rec *recorder) model {
	t.Helper()
	m := newModel(rec, core.AddOptions{}, 20)
	batch, ok := m.Init()().(tea.BatchMsg)
	if !ok {
		t.Fatal("Init should return a batch")
	}
	for _, c := range batch {
		if c == nil {
			continue
		}
		if msg, ok := c().(pingMsg); ok {
			return send(m, msg)
		}
	}
	t.Fatal("Init does not ping qBittorrent")
	return m
}

func TestPingErrorShownOnSearchScreen(t *testing.T) {
	m := startup(t, &recorder{pingErr: errors.New("connection refused\ndetails")})
	v := m.View()
	if !strings.Contains(v, "qBittorrent unreachable: connection refused") || strings.Contains(v, "details") {
		t.Fatalf("view should show the first line of the ping error:\n%s", v)
	}
	// Searching still works.
	if m = send(m, key("x"), key("enter")); m.screen != screenSearching {
		t.Fatalf("screen = %v, want searching", m.screen)
	}
}

func TestPingSuccessShowsNoError(t *testing.T) {
	v := startup(t, &recorder{}).View()
	if strings.Contains(v, "unreachable") || !strings.Contains(v, "connected to qBittorrent v5.0.4") {
		t.Fatalf("unexpected view:\n%s", v)
	}
}
