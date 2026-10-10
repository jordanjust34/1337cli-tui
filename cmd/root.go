package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"1337cli/internal/scraper"

	"github.com/spf13/cobra"
)

var (
	jsonOut     bool
	searchQuery string
	page        int
	limit       int
	mirror      string
	flareURL    string
	cookie      string
	userAgent   string
	getIndexes  []int
)

var rootCmd = &cobra.Command{
	Use:   "1337cli -s <query>",
	Short: "Search 1337x from the command line",
	Example: `  1337cli -s ubuntu
  1337cli -s "ubuntu 24.04" --page 2
  1337cli -s ubuntu -n 5
  1337cli -s ubuntu -n 50
  1337cli -s debian --flaresolverr http://localhost:8191
  1337cli -s ubuntu -n 10 --get 3          # send result #3 to qBittorrent
  1337cli -s ubuntu --get 1,4 --category linux`,
	SilenceUsage:  true,
	SilenceErrors: true,
	// Without this, Cobra treats extra words as unknown subcommands once "get" exists.
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Allow unquoted multi-word queries: `1337cli -s ubuntu 24.04`.
		query := strings.TrimSpace(strings.Join(append([]string{searchQuery}, args...), " "))
		if query == "" {
			return cmd.Help()
		}
		if page < 1 {
			return errors.New("--page must be 1 or greater")
		}

		if limit < 0 {
			return errors.New("--limit must be 0 or greater")
		}
		if jsonOut && len(getIndexes) > 0 {
			return errors.New("--json can't be combined with --get")
		}
		if len(getIndexes) > 0 && qbitURL == "" {
			return errNoQbit
		}

		c := newScraper()
		results, err := collect(c, query, page, limit)
		if err != nil {
			return explain(err)
		}
		if jsonOut {
			if results == nil {
				results = []scraper.Result{} // encode as [] rather than null
			}
			return json.NewEncoder(os.Stdout).Encode(results)
		}
		if len(results) == 0 {
			fmt.Fprintf(os.Stderr, "No results for %q.\n", query)
			return nil
		}
		printResults(results)

		if len(getIndexes) == 0 {
			return nil
		}
		var urls []string
		for _, i := range getIndexes {
			if i < 1 || i > len(results) {
				return fmt.Errorf("--get %d: no such result (have 1-%d)", i, len(results))
			}
			urls = append(urls, results[i-1].URL)
		}
		fmt.Println()
		return sendToQbit(c, urls)
	},
}

func newScraper() *scraper.Client {
	c := scraper.NewClient(mirror)
	c.FlareSolver = flareURL
	c.Cookie = cookie
	if userAgent != "" {
		c.UserAgent = userAgent
	}
	return c
}

// explain adds remediation hints to a Cloudflare challenge error.
func explain(err error) error {
	if errors.Is(err, scraper.ErrChallenge) {
		return fmt.Errorf("%w\n\n%s fronts the site with a Cloudflare challenge. Either:\n"+
			"  - run FlareSolverr and pass --flaresolverr http://localhost:8191 (or set FLARESOLVERR_URL), or\n"+
			"  - pass --cookie 'cf_clearance=...' and --user-agent copied from a browser that passed the check", err, mirror)
	}
	return err
}

// pageSize is how many entries 1337x shows per search page; a shorter page is the last one.
const pageSize = 20

// collect fetches results starting at page until it has limit entries or runs
// out of pages. A limit of 0 means a single page.
func collect(c *scraper.Client, query string, page, limit int) ([]scraper.Result, error) {
	var all []scraper.Result
	for p := page; ; p++ {
		results, err := c.Search(query, p)
		// 1337x occasionally serves an empty table mid-pagination; retry once
		// before treating an empty later page as the end of the results.
		if err == nil && len(results) == 0 && len(all) > 0 {
			results, err = c.Search(query, p)
			if err == nil && len(results) == 0 {
				fmt.Fprintf(os.Stderr, "warning: page %d returned no results; stopping at %d entries\n", p, len(all))
			}
		}
		if err != nil {
			// Keep what we already have if a later page fails.
			if len(all) > 0 {
				fmt.Fprintf(os.Stderr, "warning: stopped at page %d: %v\n", p, err)
				break
			}
			return nil, err
		}
		all = append(all, results...)
		if limit == 0 || len(all) >= limit || len(results) < pageSize {
			break
		}
	}
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}

func printResults(results []scraper.Result) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "#\tNAME\tSE\tLE\tSIZE\tDATE\tUPLOADER")
	for i, r := range results {
		fmt.Fprintf(w, "%d\t%s\t%d\t%d\t%s\t%s\t%s\n", i+1, truncate(r.Name, 70), r.Seeders, r.Leechers, r.Size, r.Date, r.Uploader)
	}
	w.Flush()

	fmt.Println()
	for i, r := range results {
		fmt.Printf("%2d  %s\n", i+1, r.URL)
	}
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func init() {
	f := rootCmd.Flags()
	f.StringVarP(&searchQuery, "search", "s", "", "search query")
	f.IntVarP(&page, "page", "p", 1, "results page to start from")
	f.IntVarP(&limit, "limit", "n", 0, "number of entries to return, fetching more pages as needed (0 = one page)")
	f.BoolVar(&jsonOut, "json", false, "print results as JSON (for scripts and 1337tui)")
	f.IntSliceVarP(&getIndexes, "get", "g", nil, "send these result numbers to qBittorrent, e.g. --get 1,3")

	f = rootCmd.PersistentFlags()
	f.StringVarP(&mirror, "mirror", "m", envOr("LEET_MIRROR", "https://1337x.to"), "1337x mirror base URL (env LEET_MIRROR)")
	f.StringVar(&flareURL, "flaresolverr", os.Getenv("FLARESOLVERR_URL"), "FlareSolverr endpoint for bypassing Cloudflare (env FLARESOLVERR_URL)")
	f.StringVar(&cookie, "cookie", os.Getenv("LEET_COOKIE"), "Cookie header to send, e.g. cf_clearance=... (env LEET_COOKIE)")
	f.StringVar(&userAgent, "user-agent", os.Getenv("LEET_USER_AGENT"), "User-Agent to send; must match the browser the cookie came from (env LEET_USER_AGENT)")
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func Execute() error {
	return rootCmd.Execute()
}
