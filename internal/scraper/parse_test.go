package scraper

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestParseSearch(t *testing.T) {
	f, err := os.Open("testdata/search.html")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	got, err := ParseSearch(f, "https://1337x.to")
	if err != nil {
		t.Fatal(err)
	}
	want := []Result{
		{Name: "Ubuntu 24.04 LTS Desktop amd64", URL: "https://1337x.to/torrent/5501234/Ubuntu-24-04-LTS-Desktop-amd64/", Seeders: 1204, Leechers: 37, Date: "Apr. 25th '24", Size: "5.7 GB", Uploader: "distro"},
		{Name: "Ubuntu Server 22.04", URL: "https://1337x.to/torrent/4400001/Ubuntu-Server-22-04/", Seeders: 88, Leechers: 2, Date: "Aug. 10th '22", Size: "1.4 GB", Uploader: "someone"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d results, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("result %d:\n got  %+v\n want %+v", i, got[i], want[i])
		}
	}
}

func TestParseSearchChallenge(t *testing.T) {
	f, err := os.Open("testdata/challenge.html")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if _, err := ParseSearch(f, "https://1337x.to"); !errors.Is(err, ErrChallenge) {
		t.Fatalf("got err %v, want ErrChallenge", err)
	}
}

func TestParseMagnet(t *testing.T) {
	f, err := os.Open("testdata/detail.html")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	got, err := ParseMagnet(f)
	if err != nil {
		t.Fatal(err)
	}
	const prefix = "magnet:?xt=urn:btih:9F9165D9A281A9B8E782CD5176BBCC8256FD1871&dn=Ubuntu+16.04.1+LTS+Desktop+64-bit&tr="
	if !strings.HasPrefix(got, prefix) {
		t.Fatalf("got %q, want prefix %q", got, prefix)
	}
}

func TestParseMagnetMissing(t *testing.T) {
	if _, err := ParseMagnet(strings.NewReader("<html><head><title>x</title></head><body></body></html>")); !errors.Is(err, ErrNoMagnet) {
		t.Fatalf("got err %v, want ErrNoMagnet", err)
	}
}
