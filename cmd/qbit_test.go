package cmd

import (
	"reflect"
	"strings"
	"testing"
)

// failReader fails the test if anything reads from it.
type failReader struct{ t *testing.T }

func (r failReader) Read([]byte) (int, error) {
	r.t.Fatal("stdin was read without a \"-\" argument")
	return 0, nil
}

func TestExpandTargets(t *testing.T) {
	stdin := strings.NewReader("  https://1337x.to/torrent/2/b/  \n\n# a comment\n\tmagnet:?xt=urn:btih:c\n   \n")
	got, err := expandTargets([]string{"https://1337x.to/torrent/1/a/", "-", "magnet:?xt=urn:btih:d"}, stdin)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"https://1337x.to/torrent/1/a/",
		"https://1337x.to/torrent/2/b/",
		"magnet:?xt=urn:btih:c",
		"magnet:?xt=urn:btih:d",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExpandTargetsOnlyDash(t *testing.T) {
	got, err := expandTargets([]string{"-"}, strings.NewReader("magnet:?xt=urn:btih:a"))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"magnet:?xt=urn:btih:a"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestExpandTargetsNoDashLeavesStdinUnread(t *testing.T) {
	args := []string{"https://1337x.to/torrent/1/a/", "magnet:?xt=urn:btih:b"}
	got, err := expandTargets(args, failReader{t})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, args) {
		t.Errorf("got %q, want %q", got, args)
	}
}

func TestExpandTargetsTwoDashes(t *testing.T) {
	_, err := expandTargets([]string{"-", "-"}, strings.NewReader("magnet:?xt=urn:btih:a\n"))
	if err == nil || !strings.Contains(err.Error(), "only be given once") {
		t.Errorf("err = %v, want a single-dash error", err)
	}
}

func TestExpandTargetsEmptyStdin(t *testing.T) {
	for _, in := range []string{"", "\n  \n# only comments\n"} {
		_, err := expandTargets([]string{"-"}, strings.NewReader(in))
		if err == nil || !strings.Contains(err.Error(), "no targets on stdin") {
			t.Errorf("stdin %q: err = %v, want a no-targets error", in, err)
		}
	}
}
