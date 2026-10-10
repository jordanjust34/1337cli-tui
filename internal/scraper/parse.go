package scraper

import (
	"errors"
	"io"
	"net/url"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// Result is a single row from a 1337x search results page.
type Result struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Seeders  int    `json:"seeders"`
	Leechers int    `json:"leechers"`
	Date     string `json:"date"`
	Size     string `json:"size"`
	Uploader string `json:"uploader"`
}

// ErrChallenge is returned when the page is a Cloudflare challenge instead of real content.
var ErrChallenge = errors.New("blocked by Cloudflare challenge")

// ParseSearch extracts results from a search results page. baseURL is used to
// turn relative torrent links into absolute URLs.
func ParseSearch(r io.Reader, baseURL string) ([]Result, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return nil, err
	}
	if isChallenge(doc) {
		return nil, ErrChallenge
	}
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}

	var results []Result
	doc.Find("table.table-list tbody tr").Each(func(_ int, tr *goquery.Selection) {
		link := tr.Find(`td.coll-1 a[href^="/torrent/"]`).First()
		href, ok := link.Attr("href")
		if !ok {
			return
		}
		ref, err := url.Parse(href)
		if err != nil {
			return
		}
		results = append(results, Result{
			Name:     strings.TrimSpace(link.Text()),
			URL:      base.ResolveReference(ref).String(),
			Seeders:  atoi(tr.Find("td.coll-2").Text()),
			Leechers: atoi(tr.Find("td.coll-3").Text()),
			Date:     strings.TrimSpace(tr.Find("td.coll-date").Text()),
			Size:     ownText(tr.Find("td.coll-4")),
			Uploader: strings.TrimSpace(tr.Find("td.coll-5").Text()),
		})
	})
	return results, nil
}

func isChallenge(doc *goquery.Document) bool {
	title := strings.TrimSpace(doc.Find("title").Text())
	return title == "Just a moment..." || title == "Attention Required! | Cloudflare"
}

// ownText returns the text directly inside s, ignoring child elements. The size
// cell also contains a hidden <span> with the seeder count for mobile layouts.
func ownText(s *goquery.Selection) string {
	var b strings.Builder
	s.Contents().Each(func(_ int, c *goquery.Selection) {
		if goquery.NodeName(c) == "#text" {
			b.WriteString(c.Text())
		}
	})
	return strings.TrimSpace(b.String())
}

func atoi(s string) int {
	n, _ := strconv.Atoi(strings.ReplaceAll(strings.TrimSpace(s), ",", ""))
	return n
}

// ErrNoMagnet is returned when a torrent page has no magnet link.
var ErrNoMagnet = errors.New("no magnet link found on page")

// ParseMagnet extracts the magnet link from a torrent detail page.
func ParseMagnet(r io.Reader) (string, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return "", err
	}
	if isChallenge(doc) {
		return "", ErrChallenge
	}
	// Class names on the download buttons are randomised, so match on the scheme.
	href, ok := doc.Find(`a[href^="magnet:"]`).First().Attr("href")
	if !ok {
		return "", ErrNoMagnet
	}
	return href, nil
}
