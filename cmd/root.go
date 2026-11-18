package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"1337cli/internal/core"

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
	getSpecs    []string
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
  1337cli -s ubuntu --get 1,4 --category linux
  1337cli -s ubuntu --get 2-5              # send results #2 to #5
  1337cli -s ubuntu -n 5 --get all`,
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
		if jsonOut && len(getSpecs) > 0 {
			return errors.New("--json can't be combined with --get")
		}
		if len(getSpecs) > 0 && qbitURL == "" {
			return errNoQbit
		}

		s := newService()
		results, err := s.Search(query, page, limit)
		if err != nil {
			return err
		}
		if jsonOut {
			if results == nil {
				results = []core.Result{} // encode as [] rather than null
			}
			return json.NewEncoder(os.Stdout).Encode(results)
		}
		if len(results) == 0 {
			fmt.Fprintf(os.Stderr, "No results for %q.\n", query)
			return nil
		}
		printResults(results)

		if len(getSpecs) == 0 {
			return nil
		}
		picks, err := parseSelection(getSpecs, len(results))
		if err != nil {
			return err
		}
		var urls []string
		for _, i := range picks {
			urls = append(urls, results[i-1].URL)
		}
		fmt.Println()
		return sendToQbit(s, urls)
	},
}

func printResults(results []core.Result) {
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
	f.BoolVar(&jsonOut, "json", false, "print results as JSON (for scripts)")
	f.StringSliceVarP(&getSpecs, "get", "g", nil, "send these results to qBittorrent: numbers, ranges or all, e.g. --get 1,3 or --get 2-5")

	f = rootCmd.PersistentFlags()
	f.StringVarP(&mirror, "mirror", "m", envOr("LEET_MIRROR", core.DefaultMirror), "1337x mirror base URL (env LEET_MIRROR)")
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
