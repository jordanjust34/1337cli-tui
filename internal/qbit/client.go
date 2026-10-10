// Package qbit is a minimal client for the qBittorrent Web API (v2).
package qbit

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Client talks to a qBittorrent Web UI, e.g. http://nas.local:8080.
type Client struct {
	BaseURL  string
	Username string
	Password string
	HTTP     *http.Client

	loggedIn bool
}

func NewClient(baseURL, username, password string) *Client {
	jar, _ := cookiejar.New(nil)
	return &Client{
		BaseURL:  strings.TrimRight(baseURL, "/"),
		Username: username,
		Password: password,
		HTTP:     &http.Client{Timeout: 30 * time.Second, Jar: jar},
	}
}

// AddOptions are optional settings for a new torrent.
type AddOptions struct {
	SavePath string
	Category string
	Paused   bool
}

// Login authenticates and stores the session cookie. It is a no-op without a
// username, for servers that whitelist the client (e.g. "bypass auth for localhost").
func (c *Client) Login() error {
	if c.loggedIn || c.Username == "" {
		return nil
	}
	body, err := c.post("/api/v2/auth/login", url.Values{
		"username": {c.Username},
		"password": {c.Password},
	})
	if err != nil {
		return err
	}
	// 4.x answers "Ok." / "Fails." with 200; 5.x answers 204 on success and 401 on failure.
	if strings.TrimSpace(body) == "Fails." {
		return fmt.Errorf("qbittorrent login failed: check username and password")
	}
	c.loggedIn = true
	return nil
}

// AddMagnet queues a magnet link for download.
func (c *Client) AddMagnet(magnet string, opts AddOptions) error {
	if err := c.Login(); err != nil {
		return err
	}
	form := url.Values{"urls": {magnet}}
	if opts.SavePath != "" {
		form.Set("savepath", opts.SavePath)
	}
	if opts.Category != "" {
		form.Set("category", opts.Category)
		// A category's save path (e.g. a Jellyfin library folder) is only applied
		// under automatic torrent management, which is off by default. An explicit
		// save path wins, so only enable it when none was given.
		if opts.SavePath == "" {
			form.Set("autoTMM", "true")
		}
	}
	// qBittorrent 5.x renamed "paused" to "stopped"; send both so either version honours it.
	state := fmt.Sprint(opts.Paused)
	form.Set("paused", state)
	form.Set("stopped", state)

	body, err := c.post("/api/v2/torrents/add", form)
	if err != nil {
		return err
	}
	if strings.TrimSpace(body) == "Fails." {
		return fmt.Errorf("qbittorrent rejected the torrent (invalid magnet or already added)")
	}
	return nil
}

// Version returns the qBittorrent application version; useful as a connectivity check.
func (c *Client) Version() (string, error) {
	if err := c.Login(); err != nil {
		return "", err
	}
	req, err := http.NewRequest(http.MethodGet, c.BaseURL+"/api/v2/app/version", nil)
	if err != nil {
		return "", err
	}
	body, err := c.do(req)
	return strings.TrimSpace(body), err
}

func (c *Client) post(path string, form url.Values) (string, error) {
	req, err := http.NewRequest(http.MethodPost, c.BaseURL+path, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return c.do(req)
}

func (c *Client) do(req *http.Request) (string, error) {
	// qBittorrent's CSRF protection rejects requests whose Referer/Origin
	// doesn't match the host it's served on.
	req.Header.Set("Referer", c.BaseURL)
	req.Header.Set("Origin", c.BaseURL)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("qbittorrent: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return string(body), nil
	case resp.StatusCode == http.StatusConflict:
		return "", fmt.Errorf("qbittorrent: torrent is already added")
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return "", fmt.Errorf("qbittorrent: %s %s: %s (wrong credentials, IP banned, login required, or the URL's port differs from qBittorrent's own WebUI port, e.g. a remapped Docker port)", req.Method, req.URL.Path, resp.Status)
	default:
		return "", fmt.Errorf("qbittorrent: %s %s: %s %s", req.Method, req.URL.Path, resp.Status, strings.TrimSpace(string(body)))
	}
}

// Category is a qBittorrent category and the folder its torrents save to.
type Category struct {
	Name     string `json:"name"`
	SavePath string `json:"savePath"`
}

// Categories returns the server's categories, sorted by name.
func (c *Client) Categories() ([]Category, error) {
	if err := c.Login(); err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodGet, c.BaseURL+"/api/v2/torrents/categories", nil)
	if err != nil {
		return nil, err
	}
	body, err := c.do(req)
	if err != nil {
		return nil, err
	}
	var byName map[string]Category
	if err := json.Unmarshal([]byte(body), &byName); err != nil {
		return nil, fmt.Errorf("qbittorrent: decoding categories: %w", err)
	}
	cats := make([]Category, 0, len(byName))
	for _, cat := range byName {
		cats = append(cats, cat)
	}
	sort.Slice(cats, func(i, j int) bool { return cats[i].Name < cats[j].Name })
	return cats, nil
}
