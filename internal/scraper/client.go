package scraper

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const DefaultUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36"

// Client fetches pages from a 1337x mirror, either directly or through FlareSolverr.
type Client struct {
	BaseURL     string // e.g. https://1337x.to
	UserAgent   string
	Cookie      string // raw Cookie header, e.g. "cf_clearance=..."
	FlareSolver string // FlareSolverr endpoint, e.g. http://localhost:8191/v1
	HTTP        *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL:   strings.TrimRight(baseURL, "/"),
		UserAgent: DefaultUserAgent,
		HTTP:      &http.Client{Timeout: 30 * time.Second},
	}
}

// Search returns the results for query on the given page (1-based).
func (c *Client) Search(query string, page int) ([]Result, error) {
	u := fmt.Sprintf("%s/search/%s/%d/", c.BaseURL, url.PathEscape(query), page)
	body, err := c.get(u)
	if err != nil {
		return nil, err
	}
	return ParseSearch(bytes.NewReader(body), c.BaseURL)
}

// Magnet fetches a torrent detail page and returns its magnet link.
func (c *Client) Magnet(pageURL string) (string, error) {
	body, err := c.get(pageURL)
	if err != nil {
		return "", err
	}
	return ParseMagnet(bytes.NewReader(body))
}

func (c *Client) get(u string) ([]byte, error) {
	if c.FlareSolver != "" {
		return c.getViaFlareSolverr(u)
	}

	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	if c.Cookie != "" {
		req.Header.Set("Cookie", c.Cookie)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	// Cloudflare serves its challenge with a 403/503; let the parser detect it
	// so the caller gets ErrChallenge rather than a bare status code.
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusServiceUnavailable {
		return nil, fmt.Errorf("GET %s: %s", u, resp.Status)
	}
	return body, nil
}

type flareRequest struct {
	Cmd        string `json:"cmd"`
	URL        string `json:"url"`
	MaxTimeout int    `json:"maxTimeout"`
}

type flareResponse struct {
	Status   string `json:"status"`
	Message  string `json:"message"`
	Solution struct {
		Status   int    `json:"status"`
		Response string `json:"response"`
	} `json:"solution"`
}

func (c *Client) getViaFlareSolverr(u string) ([]byte, error) {
	endpoint := strings.TrimRight(c.FlareSolver, "/")
	if !strings.HasSuffix(endpoint, "/v1") {
		endpoint += "/v1"
	}
	payload, _ := json.Marshal(flareRequest{Cmd: "request.get", URL: u, MaxTimeout: 60000})

	// FlareSolverr drives a real browser, so allow more time than direct requests.
	hc := &http.Client{Timeout: 90 * time.Second}
	resp, err := hc.Post(endpoint, "application/json", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("flaresolverr: %w", err)
	}
	defer resp.Body.Close()

	var fr flareResponse
	if err := json.NewDecoder(resp.Body).Decode(&fr); err != nil {
		return nil, fmt.Errorf("flaresolverr: decoding response: %w", err)
	}
	if fr.Status != "ok" {
		return nil, fmt.Errorf("flaresolverr: %s", fr.Message)
	}
	return []byte(fr.Solution.Response), nil
}
