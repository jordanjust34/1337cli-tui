package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	best        bool
	interactive bool
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
  1337cli -s ubuntu -n 5 --get all
  1337cli -s ubuntu --best -c linux        # send the most-seeded result
  1337cli -s ubuntu -i                     # choose from the table at a prompt`,
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
		if err := checkSendFlags(); err != nil {
			return err
		}
		submits := len(getSpecs) > 0 || best || interactive
		if submits && qbitURL == "" {
			return errNoQbit
		}

		s := newService()
		if submits {
			// Fail before a search that can take ~15 s per page through FlareSolverr.
			if _, err := s.Ping(); err != nil {
				return err
			}
		}
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

		if !submits {
			return nil
		}
		opts := addOptions()
		targets, category, err := chooseTargets(s, cmd.InOrStdin(), results)
		if err != nil || len(targets) == 0 {
			return err
		}
		if category != "" {
			opts.Category = category // this run only
		}
		fmt.Println()
		return sendToQbit(s, targets, opts)
	},
}

// checkSendFlags rejects combinations of --json, --get, --best and
// --interactive, which each decide differently what happens after the search.
func checkSendFlags() error {
	set := []struct {
		name string
		on   bool
	}{{"--json", jsonOut}, {"--get", len(getSpecs) > 0}, {"--best", best}, {"--interactive", interactive}}
	for i, a := range set {
		for _, b := range set[i+1:] {
			if a.on && b.on {
				return fmt.Errorf("%s can't be combined with %s", a.name, b.name)
			}
		}
	}
	if interactive && !stdinIsTerminal() {
		return errors.New("--interactive needs a terminal; use --get in scripts")
	}
	return nil
}

// chooseTargets returns the page URLs to send, picked by --get, --best or
// --interactive, and the category chosen at the prompt (empty for none).
// No targets and no error means the user chose to send nothing.
func chooseTargets(s *core.Service, stdin io.Reader, results []core.Result) ([]string, string, error) {
	switch {
	case len(getSpecs) > 0:
		picks, err := parseSelection(getSpecs, len(results))
		if err != nil {
			return nil, "", err
		}
		var urls []string
		for _, i := range picks {
			urls = append(urls, results[i-1].URL)
		}
		return urls, "", nil
	case best:
		var urls []string
		for _, r := range core.Best(results, 1) {
			urls = append(urls, r.URL)
		}
		return urls, "", nil
	case interactive:
		fmt.Fprintln(os.Stderr)
		return pickTargets(stdin, os.Stderr, results, s.Categories, qbitCategory == "" && qbitSavePath == "")
	}
	return nil, "", nil
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
	f.BoolVar(&best, "best", false, "send the result with the most seeders to qBittorrent")
	f.BoolVarP(&interactive, "interactive", "i", false, "after the table, ask which results to send (and the category, if none is set)")

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
