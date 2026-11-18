package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"

	"1337cli/internal/core"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// resetFlags puts every flag of every command back to its default and clears
// Changed, since cmd/ keeps flag values in package variables that would
// otherwise leak between tests. Defaults read from the environment (and the
// user's config file) are cleared so a developer's QBIT_CATEGORY etc. can't
// change test results.
func resetFlags(t *testing.T) {
	t.Helper()
	reset := func(f *pflag.Flag) {
		if sv, ok := f.Value.(pflag.SliceValue); ok {
			sv.Replace(nil)
		} else if err := f.Value.Set(f.DefValue); err != nil {
			t.Fatalf("reset --%s: %v", f.Name, err)
		}
		f.Changed = false
	}
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		c.Flags().VisitAll(reset)
		c.PersistentFlags().VisitAll(reset)
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(rootCmd)
	mirror = core.DefaultMirror
	flareURL, cookie, userAgent = "", "", ""
	qbitURL, qbitUser, qbitPass, qbitSavePath, qbitCategory = "", "", "", "", ""
	stdinIsTerminal = func() bool { return true }
	rootCmd.SetIn(strings.NewReader(""))
}

// run executes rootCmd with args after resetting flags.
func run(t *testing.T, args ...string) error {
	t.Helper()
	resetFlags(t)
	t.Cleanup(func() { resetFlags(t) })
	rootCmd.SetArgs(args)
	return rootCmd.Execute()
}

// fakeSite serves one search page whose rows have the given seeder counts
// (row i is torrent i, named r<i>), and detail pages with a magnet per torrent.
// searches counts search requests.
func fakeSite(t *testing.T, seeders ...int) (srv *httptest.Server, searches *int) {
	var mu sync.Mutex
	n := 0
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/search/"):
			mu.Lock()
			n++
			mu.Unlock()
			var b strings.Builder
			b.WriteString(`<html><head><title>1337x</title></head><body><table class="table-list"><tbody>`)
			for i, s := range seeders {
				fmt.Fprintf(&b, `<tr><td class="coll-1"><a href="/torrent/%d/r%d/">r%d</a></td><td class="coll-2">%d</td><td class="coll-3">0</td><td class="coll-date">x</td><td class="coll-4">1 GB</td><td class="coll-5">u</td></tr>`, i+1, i+1, i+1, s)
			}
			b.WriteString(`</tbody></table></body></html>`)
			w.Write([]byte(b.String()))
		case strings.HasPrefix(r.URL.Path, "/torrent/"):
			name := strings.Split(strings.Trim(r.URL.Path, "/"), "/")[2]
			fmt.Fprintf(w, `<html><body><a href="magnet:?xt=urn:btih:%s&dn=%s">m</a></body></html>`, name, name)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &n
}

// add is one torrents/add request seen by fakeQbit.
type add struct{ URLs, Category string }

// fakeQbit accepts the login "admin"/"secret", answers app/version and lists
// two categories, and records each torrents/add.
func fakeQbit(t *testing.T) (srv *httptest.Server, adds *[]add) {
	var mu sync.Mutex
	var got []add
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		switch r.URL.Path {
		case "/api/v2/auth/login":
			if r.Form.Get("password") != "secret" {
				w.Write([]byte("Fails."))
				return
			}
			w.Write([]byte("Ok."))
		case "/api/v2/app/version":
			w.Write([]byte("v4.6.0"))
		case "/api/v2/torrents/categories":
			w.Write([]byte(`{"linux":{"name":"linux","savePath":"/srv/linux"},"movies":{"name":"movies","savePath":"/srv/movies"}}`))
		case "/api/v2/torrents/add":
			mu.Lock()
			got = append(got, add{r.Form.Get("urls"), r.Form.Get("category")})
			mu.Unlock()
			w.Write([]byte("Ok."))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

func magnet(name string) string { return "magnet:?xt=urn:btih:" + name + "&dn=" + name }

func TestRootGetSendsRange(t *testing.T) {
	site, _ := fakeSite(t, 5, 9, 1)
	qb, adds := fakeQbit(t)
	err := run(t, "-s", "q", "-m", site.URL, "--qbit-url", qb.URL, "--qbit-user", "admin", "--qbit-pass", "secret", "-g", "1-2", "-c", "linux")
	if err != nil {
		t.Fatal(err)
	}
	want := []add{{magnet("r1"), "linux"}, {magnet("r2"), "linux"}}
	if !reflect.DeepEqual(*adds, want) {
		t.Errorf("adds = %v, want %v", *adds, want)
	}
}

func TestRootBestSendsMostSeeded(t *testing.T) {
	site, _ := fakeSite(t, 5, 9, 1)
	qb, adds := fakeQbit(t)
	if err := run(t, "-s", "q", "-m", site.URL, "--qbit-url", qb.URL, "--best"); err != nil {
		t.Fatal(err)
	}
	if want := []add{{magnet("r2"), ""}}; !reflect.DeepEqual(*adds, want) {
		t.Errorf("adds = %v, want %v", *adds, want)
	}
}

func TestRootInteractivePicksCategoryForThisRun(t *testing.T) {
	site, _ := fakeSite(t, 5, 9, 1)
	qb, adds := fakeQbit(t)
	resetFlags(t)
	t.Cleanup(func() { resetFlags(t) })
	rootCmd.SetIn(strings.NewReader("3,1\n2\n"))
	rootCmd.SetArgs([]string{"-s", "q", "-m", site.URL, "--qbit-url", qb.URL, "-i"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	want := []add{{magnet("r3"), "movies"}, {magnet("r1"), "movies"}}
	if !reflect.DeepEqual(*adds, want) {
		t.Errorf("adds = %v, want %v", *adds, want)
	}
	if qbitCategory != "" {
		t.Errorf("the prompt's category leaked into the flag: %q", qbitCategory)
	}
}

func TestRootInteractiveQuitSendsNothing(t *testing.T) {
	site, _ := fakeSite(t, 5)
	qb, adds := fakeQbit(t)
	if err := run(t, "-s", "q", "-m", site.URL, "--qbit-url", qb.URL, "-i", "-c", "linux"); err != nil {
		t.Fatal(err) // stdin is empty: EOF quits with exit 0
	}
	if len(*adds) != 0 {
		t.Errorf("adds = %v, want none", *adds)
	}
}

func TestRootPingFailsBeforeSearch(t *testing.T) {
	site, searches := fakeSite(t, 5)
	qb, _ := fakeQbit(t)
	err := run(t, "-s", "q", "-m", site.URL, "--qbit-url", qb.URL, "--qbit-user", "admin", "--qbit-pass", "wrong", "--best")
	if err == nil || !strings.Contains(err.Error(), "login failed") {
		t.Fatalf("err = %v, want a login error", err)
	}
	if *searches != 0 {
		t.Errorf("searched %d times before failing; want 0", *searches)
	}
}

func TestRootFlagConflicts(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--json", "-g", "1"}, "--json can't be combined with --get"},
		{[]string{"--json", "--best"}, "--json can't be combined with --best"},
		{[]string{"--json", "-i"}, "--json can't be combined with --interactive"},
		{[]string{"-g", "1", "--best"}, "--get can't be combined with --best"},
		{[]string{"-g", "1", "-i"}, "--get can't be combined with --interactive"},
		{[]string{"--best", "-i"}, "--best can't be combined with --interactive"},
		{[]string{"--best"}, errNoQbit.Error()},
		{[]string{"-i"}, errNoQbit.Error()},
	} {
		err := run(t, append([]string{"-s", "q", "-m", "http://127.0.0.1:1"}, tc.args...)...)
		if err == nil || err.Error() != tc.want {
			t.Errorf("%v: err = %v, want %q", tc.args, err, tc.want)
		}
	}
}

func TestRootInteractiveNeedsTerminal(t *testing.T) {
	resetFlags(t)
	t.Cleanup(func() { resetFlags(t) })
	stdinIsTerminal = func() bool { return false }
	rootCmd.SetArgs([]string{"-s", "q", "--qbit-url", "http://127.0.0.1:1", "-i"})
	want := "--interactive needs a terminal; use --get in scripts"
	if err := rootCmd.Execute(); err == nil || err.Error() != want {
		t.Errorf("err = %v, want %q", err, want)
	}
}

// stdout captures what f writes to os.Stdout, where the commands print results.
func stdout(t *testing.T, f func() error) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = orig }()
	ferr := f()
	w.Close()
	out, _ := io.ReadAll(r)
	return string(out), ferr
}

func TestRootJSON(t *testing.T) {
	site, _ := fakeSite(t, 7)
	out, err := stdout(t, func() error { return run(t, "-s", "q", "-m", site.URL, "--json") })
	if err != nil {
		t.Fatal(err)
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(out), &rows); err != nil || len(rows) != 1 {
		t.Fatalf("output %q: %v", out, err)
	}
	for _, k := range []string{"name", "url", "seeders", "leechers", "date", "size", "uploader"} {
		if _, ok := rows[0][k]; !ok {
			t.Errorf("JSON result has no %q: %v", k, rows[0])
		}
	}

	// No results encode as [] rather than null, so scripts can always iterate.
	empty, _ := fakeSite(t)
	out, err = stdout(t, func() error { return run(t, "-s", "q", "-m", empty.URL, "--json") })
	if err != nil || strings.TrimSpace(out) != "[]" {
		t.Fatalf("empty search: output %q, err %v; want []", out, err)
	}
}

func TestRootValidatesPageAndLimit(t *testing.T) {
	for args, want := range map[string]string{
		"-p 0":  "--page must be 1 or greater",
		"-n -1": "--limit must be 0 or greater",
	} {
		err := run(t, append([]string{"-s", "q", "-m", "http://127.0.0.1:1"}, strings.Fields(args)...)...)
		if err == nil || err.Error() != want {
			t.Errorf("%s: err = %v, want %q", args, err, want)
		}
	}
}

func TestGetFromStdinReportsFailures(t *testing.T) {
	site, _ := fakeSite(t)
	qb, adds := fakeQbit(t)
	resetFlags(t)
	t.Cleanup(func() { resetFlags(t) })
	// The second target 404s; the batch must carry on and count it.
	rootCmd.SetIn(strings.NewReader(site.URL + "/torrent/1/one/\n# comment\n" + site.URL + "/nope\n"))
	rootCmd.SetArgs([]string{"get", "-", magnet("three"), "--qbit-url", qb.URL, "-c", "linux"})
	err := rootCmd.Execute()
	if err == nil || err.Error() != "1 of 3 torrent(s) failed" {
		t.Fatalf("err = %v, want the failure count", err)
	}
	want := []add{{magnet("one"), "linux"}, {magnet("three"), "linux"}}
	if !reflect.DeepEqual(*adds, want) {
		t.Errorf("adds = %v, want %v", *adds, want)
	}
}

func TestCategoriesJSON(t *testing.T) {
	qb, _ := fakeQbit(t)
	out, err := stdout(t, func() error { return run(t, "categories", "--json", "--qbit-url", qb.URL) })
	if err != nil {
		t.Fatal(err)
	}
	want := `[{"name":"linux","savePath":"/srv/linux"},{"name":"movies","savePath":"/srv/movies"}]`
	if strings.TrimSpace(out) != want {
		t.Errorf("categories --json = %s, want %s", out, want)
	}
	if err := run(t, "categories"); err != errNoQbit {
		t.Errorf("without a server: err = %v, want errNoQbit", err)
	}
}
