package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"1337cli/internal/scraper"
)

// searchPage renders a 1337x-style results table with n rows numbered from first.
func searchPage(first, n int) string {
	var b strings.Builder
	b.WriteString(`<html><head><title>1337x</title></head><body><table class="table-list"><tbody>`)
	for i := first; i < first+n; i++ {
		fmt.Fprintf(&b, `<tr><td class="coll-1"><a href="/torrent/%d/r%d/">r%d</a></td><td class="coll-2">1</td><td class="coll-3">0</td><td class="coll-date">x</td><td class="coll-4">1 GB</td><td class="coll-5">u</td></tr>`, i, i, i)
	}
	b.WriteString(`</tbody></table></body></html>`)
	return b.String()
}

// fake1337x serves search page p from pages(p, hit), where hit counts requests
// for that page, and detail pages with a magnet named after the torrent.
func fake1337x(t *testing.T, pages func(page, hit int) (string, int)) *httptest.Server {
	var mu sync.Mutex
	hits := map[int]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p int
		if _, err := fmt.Sscanf(r.URL.Path[strings.LastIndex(strings.TrimSuffix(r.URL.Path, "/"), "/")+1:], "%d", &p); err == nil && strings.HasPrefix(r.URL.Path, "/search/") {
			mu.Lock()
			hits[p]++
			h := hits[p]
			mu.Unlock()
			body, status := pages(p, h)
			w.WriteHeader(status)
			w.Write([]byte(body))
			return
		}
		if strings.HasPrefix(r.URL.Path, "/torrent/") {
			name := strings.Split(strings.Trim(r.URL.Path, "/"), "/")[2]
			if name == "nomagnet" {
				w.Write([]byte(`<html><head><title>x</title></head></html>`))
				return
			}
			fmt.Fprintf(w, `<html><body><a href="magnet:?xt=urn:btih:%s&dn=%s">m</a></body></html>`, name, name)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func names(rs []Result) []string {
	var out []string
	for _, r := range rs {
		out = append(out, r.Name)
	}
	return out
}

func TestSearchPagingAndLimit(t *testing.T) {
	srv := fake1337x(t, func(p, _ int) (string, int) { return searchPage((p-1)*20+1, 20), 200 })
	s := New(Config{Mirror: srv.URL})

	got, err := s.Search("q", 1, 23)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 23 || got[0].Name != "r1" || got[22].Name != "r23" {
		t.Fatalf("limit 23: got %d results %v", len(got), names(got))
	}

	got, err = s.Search("q", 1, 0)
	if err != nil || len(got) != 20 {
		t.Fatalf("limit 0: got %d results, err %v; want one page", len(got), err)
	}

	if _, err := s.Search("q", 0, 0); err == nil {
		t.Error("page 0 should be an error")
	}
	if _, err := s.Search("q", 1, -1); err == nil {
		t.Error("negative limit should be an error")
	}
}

func TestSearchRetriesEmptyPageAndKeepsResults(t *testing.T) {
	// Page 2 is empty the first time and has one row on retry.
	srv := fake1337x(t, func(p, hit int) (string, int) {
		if p == 1 {
			return searchPage(1, 20), 200
		}
		if hit == 1 {
			return searchPage(0, 0), 200
		}
		return searchPage(21, 1), 200
	})
	var warnings []string
	s := New(Config{Mirror: srv.URL})
	s.Warn = func(msg string) { warnings = append(warnings, msg) }
	got, err := s.Search("q", 1, 40)
	if err != nil || len(got) != 21 {
		t.Fatalf("got %d results, err %v; want 21", len(got), err)
	}

	// A failing later page keeps what was collected and warns.
	srv = fake1337x(t, func(p, _ int) (string, int) {
		if p == 1 {
			return searchPage(1, 20), 200
		}
		return "boom", 500
	})
	s = New(Config{Mirror: srv.URL})
	s.Warn = func(msg string) { warnings = append(warnings, msg) }
	got, err = s.Search("q", 1, 40)
	if err != nil || len(got) != 20 || len(warnings) != 1 {
		t.Fatalf("got %d results, err %v, warnings %q; want 20 results and one warning", len(got), err, warnings)
	}

	// A first-page error reaches the caller, with hints for a challenge.
	srv = fake1337x(t, func(int, int) (string, int) {
		return `<html><head><title>Just a moment...</title></head></html>`, 403
	})
	_, err = New(Config{Mirror: srv.URL}).Search("q", 1, 0)
	if !errors.Is(err, scraper.ErrChallenge) || !strings.Contains(err.Error(), "FLARESOLVERR_URL") {
		t.Fatalf("err = %v, want ErrChallenge with remediation", err)
	}
}

// TestResultJSONFields pins the field names scripts parse from `1337cli --json`.
func TestResultJSONFields(t *testing.T) {
	b, _ := json.Marshal(Result{})
	var m map[string]any
	json.Unmarshal(b, &m)
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	want := []string{"date", "leechers", "name", "seeders", "size", "uploader", "url"}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("Result JSON keys = %v, want %v", keys, want)
	}
	b, _ = json.Marshal(Category{Name: "movies", SavePath: "/m"})
	if string(b) != `{"name":"movies","savePath":"/m"}` {
		t.Fatalf("Category JSON = %s", b)
	}
}

func TestSubmitContinuesPastFailures(t *testing.T) {
	site := fake1337x(t, nil)
	var added []string
	var form map[string]string
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.URL.Path != "/api/v2/torrents/add" {
			http.NotFound(w, r)
			return
		}
		added = append(added, r.Form.Get("urls"))
		form = map[string]string{"category": r.Form.Get("category"), "stopped": r.Form.Get("stopped")}
		w.Write([]byte("Ok."))
	}))
	defer qb.Close()

	s := New(Config{Mirror: site.URL, QbitURL: qb.URL})
	targets := []string{
		site.URL + "/torrent/1/one/",
		site.URL + "/torrent/2/nomagnet/",
		"magnet:?xt=urn:btih:three&dn=Three",
	}
	var progressed int
	outs, err := s.Submit(targets, AddOptions{Category: "movies", Paused: true}, func(Outcome) { progressed++ })
	if err != nil {
		t.Fatal(err)
	}
	if Failed(outs) != 1 || outs[1].Err == nil || progressed != 3 {
		t.Fatalf("outcomes = %+v, progress calls %d", outs, progressed)
	}
	if outs[0].Name != "one" || outs[2].Name != "Three" {
		t.Errorf("names = %q, %q", outs[0].Name, outs[2].Name)
	}
	want := []string{"magnet:?xt=urn:btih:one&dn=one", "magnet:?xt=urn:btih:three&dn=Three"}
	if !reflect.DeepEqual(added, want) {
		t.Fatalf("added %q, want %q", added, want)
	}
	if form["category"] != "movies" || form["stopped"] != "true" {
		t.Errorf("last add form = %v", form)
	}

	if _, err := New(Config{}).Submit(targets, AddOptions{}, nil); !errors.Is(err, ErrNoQbit) {
		t.Fatalf("err = %v, want ErrNoQbit", err)
	}
}

func TestBest(t *testing.T) {
	in := []Result{
		{Name: "a", Seeders: 5},
		{Name: "b", Seeders: 9},
		{Name: "c", Seeders: 5},
		{Name: "d", Seeders: 1},
	}
	orig := append([]Result(nil), in...)

	for _, tc := range []struct {
		n    int
		want []string
	}{
		{1, []string{"b"}},
		{3, []string{"b", "a", "c"}}, // a and c tie; source order kept
		{10, []string{"b", "a", "c", "d"}},
		{0, []string{"b"}},
		{-2, []string{"b"}},
	} {
		if got := names(Best(in, tc.n)); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("Best(n=%d) = %v, want %v", tc.n, got, tc.want)
		}
	}
	if !reflect.DeepEqual(in, orig) {
		t.Errorf("Best reordered the input: %v", names(in))
	}
	if got := Best(nil, 3); len(got) != 0 {
		t.Errorf("Best(nil) = %v, want empty", got)
	}
}

func TestPing(t *testing.T) {
	qb := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/auth/login":
			r.ParseForm()
			if r.Form.Get("password") != "secret" {
				w.Write([]byte("Fails."))
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: "s"})
			w.Write([]byte("Ok."))
		case "/api/v2/app/version":
			w.Write([]byte("v5.0.0"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer qb.Close()

	v, err := New(Config{QbitURL: qb.URL, QbitUser: "admin", QbitPass: "secret"}).Ping()
	if err != nil || v != "v5.0.0" {
		t.Fatalf("Ping = %q, %v; want v5.0.0", v, err)
	}
	if _, err := New(Config{QbitURL: qb.URL, QbitUser: "admin", QbitPass: "wrong"}).Ping(); err == nil {
		t.Error("wrong password should be an error")
	}
	if _, err := New(Config{}).Ping(); !errors.Is(err, ErrNoQbit) {
		t.Fatalf("err = %v, want ErrNoQbit", err)
	}
}
