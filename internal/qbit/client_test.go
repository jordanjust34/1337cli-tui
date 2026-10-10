package qbit

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAddMagnet(t *testing.T) {
	var added map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Referer") == "" {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		r.ParseForm()
		switch r.URL.Path {
		case "/api/v2/auth/login":
			if r.Form.Get("username") != "admin" || r.Form.Get("password") != "secret" {
				w.Write([]byte("Fails."))
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: "abc", Path: "/"})
			w.Write([]byte("Ok."))
		case "/api/v2/torrents/add":
			if c, err := r.Cookie("SID"); err != nil || c.Value != "abc" {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			added = map[string]string{
				"urls":     r.Form.Get("urls"),
				"savepath": r.Form.Get("savepath"),
				"category": r.Form.Get("category"),
				"paused":   r.Form.Get("paused"),
				"stopped":  r.Form.Get("stopped"),
				"autoTMM":  r.Form.Get("autoTMM"),
			}
			w.Write([]byte("Ok."))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "admin", "secret")
	if err := c.AddMagnet("magnet:?xt=urn:btih:abc", AddOptions{SavePath: "/downloads", Category: "linux"}); err != nil {
		t.Fatal(err)
	}
	// An explicit save path must not be overridden by the category's path.
	want := map[string]string{"urls": "magnet:?xt=urn:btih:abc", "savepath": "/downloads", "category": "linux", "paused": "false", "stopped": "false", "autoTMM": ""}
	for k, v := range want {
		if added[k] != v {
			t.Errorf("%s = %q, want %q", k, added[k], v)
		}
	}

	// A category alone turns on autoTMM so the category's save path applies.
	if err := c.AddMagnet("magnet:?xt=urn:btih:def", AddOptions{Category: "movies"}); err != nil {
		t.Fatal(err)
	}
	if added["autoTMM"] != "true" || added["savepath"] != "" {
		t.Errorf("category only: autoTMM = %q, savepath = %q; want \"true\", \"\"", added["autoTMM"], added["savepath"])
	}

	bad := NewClient(srv.URL, "admin", "wrong")
	if err := bad.AddMagnet("magnet:?xt=urn:btih:abc", AddOptions{}); err == nil {
		t.Fatal("expected login failure")
	}
}
