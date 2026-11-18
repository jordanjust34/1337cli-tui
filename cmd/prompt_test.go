package cmd

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"1337cli/internal/core"
)

var promptResults = []core.Result{
	{Name: "one", URL: "https://1337x.to/torrent/1/one/"},
	{Name: "two", URL: "https://1337x.to/torrent/2/two/"},
	{Name: "three", URL: "https://1337x.to/torrent/3/three/"},
}

func twoCats() ([]core.Category, error) {
	return []core.Category{{Name: "linux", SavePath: "/srv/linux"}, {Name: "movies", SavePath: "/srv/movies"}}, nil
}

func TestPickTargetsReasksThenPicksCategory(t *testing.T) {
	var out strings.Builder
	targets, cat, err := pickTargets(strings.NewReader("9\n1,3\n2\n"), &out, promptResults, twoCats, true)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{promptResults[0].URL, promptResults[2].URL}
	if !reflect.DeepEqual(targets, want) || cat != "movies" {
		t.Fatalf("got %q, category %q; want %q, movies", targets, cat, want)
	}
	text := out.String()
	if n := strings.Count(text, "no such result (have 1-3)"); n != 1 {
		t.Errorf("re-prompt error shown %d times, want 1:\n%s", n, text)
	}
	if n := strings.Count(text, "Send which?"); n != 2 {
		t.Errorf("selection prompt shown %d times, want 2:\n%s", n, text)
	}
	if strings.Contains(text, "--get") {
		t.Errorf("prompt errors shouldn't mention the flag:\n%s", text)
	}
	if !strings.Contains(text, "/srv/movies") || strings.Count(text, "Category?") != 1 {
		t.Errorf("category list or prompt missing:\n%s", text)
	}
}

func TestPickTargetsBadCategoryReasks(t *testing.T) {
	var out strings.Builder
	targets, cat, err := pickTargets(strings.NewReader("2\n5\nx\n1\n"), &out, promptResults, twoCats, true)
	if err != nil || len(targets) != 1 || cat != "linux" {
		t.Fatalf("got %q, %q, %v", targets, cat, err)
	}
	if n := strings.Count(out.String(), "Category?"); n != 3 {
		t.Errorf("category prompt shown %d times, want 3", n)
	}
}

func TestPickTargetsQuit(t *testing.T) {
	// Enter, EOF, blanks, and EOF at the category prompt all send nothing.
	for _, in := range []string{"\n", "", "   \n", "1\n"} {
		targets, cat, err := pickTargets(strings.NewReader(in), &strings.Builder{}, promptResults, twoCats, true)
		if err != nil || targets != nil || cat != "" {
			t.Errorf("input %q: got %q, %q, %v; want nothing", in, targets, cat, err)
		}
	}
}

func TestPickTargetsEnterKeepsServerDefault(t *testing.T) {
	targets, cat, err := pickTargets(strings.NewReader("all\n\n"), &strings.Builder{}, promptResults, twoCats, true)
	if err != nil || len(targets) != 3 || cat != "" {
		t.Fatalf("got %q, %q, %v", targets, cat, err)
	}
}

func TestPickTargetsCategoriesError(t *testing.T) {
	var out strings.Builder
	cats := func() ([]core.Category, error) { return nil, errors.New("connection refused") }
	targets, cat, err := pickTargets(strings.NewReader("2\n"), &out, promptResults, cats, true)
	if err != nil || !reflect.DeepEqual(targets, []string{promptResults[1].URL}) || cat != "" {
		t.Fatalf("got %q, %q, %v", targets, cat, err)
	}
	if !strings.Contains(out.String(), "warning: can't list categories (connection refused)") {
		t.Errorf("no warning in:\n%s", out.String())
	}
	if strings.Contains(out.String(), "Category?") {
		t.Errorf("asked for a category after the listing failed:\n%s", out.String())
	}
}

func TestPickTargetsNoCategoryNeeded(t *testing.T) {
	var out strings.Builder
	cats := func() ([]core.Category, error) { t.Fatal("categories listed although one is set"); return nil, nil }
	targets, cat, err := pickTargets(strings.NewReader("2-3\n"), &out, promptResults, cats, false)
	if err != nil || len(targets) != 2 || cat != "" {
		t.Fatalf("got %q, %q, %v", targets, cat, err)
	}
	if strings.Contains(out.String(), "Category?") {
		t.Errorf("asked for a category:\n%s", out.String())
	}
}
