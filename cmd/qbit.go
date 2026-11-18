package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"1337cli/internal/core"

	"github.com/spf13/cobra"
)

var (
	qbitURL      string
	qbitUser     string
	qbitPass     string
	qbitSavePath string
	qbitCategory string
	qbitPaused   bool
)

var errNoQbit = errors.New("no qBittorrent server configured: pass --qbit-url http://host:8080, or set QBIT_URL in the environment or " + configPath)

var getCmd = &cobra.Command{
	Use:   "get <torrent-page-url|magnet|->...",
	Short: "Send 1337x torrents to qBittorrent and start downloading",
	Long: `Send 1337x torrents to qBittorrent and start downloading.

A "-" argument reads more targets from stdin, one per line (blank lines and
lines starting with # are skipped), so search output can be piped in.`,
	Example: `  1337cli get https://1337x.to/torrent/1699436/Ubuntu-16-04-1-LTS-Desktop-64-bit/
  1337cli get 'magnet:?xt=urn:btih:...' --save-path /downloads/linux
  1337cli -s ubuntu --json | jq -r '.[0].url' | 1337cli get - -c linux`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		targets, err := expandTargets(args, cmd.InOrStdin())
		if err != nil {
			return err
		}
		return sendToQbit(newService(), targets)
	},
}

// expandTargets replaces a "-" argument with the targets read from stdin, one
// per line, keeping the order of the other arguments. Stdin is only read if
// "-" is present, and "-" is allowed once, since a second one would always be
// empty.
func expandTargets(args []string, stdin io.Reader) ([]string, error) {
	var out []string
	dashes := 0
	for _, a := range args {
		if a != "-" {
			out = append(out, a)
			continue
		}
		if dashes++; dashes > 1 {
			return nil, errors.New(`get: "-" (read targets from stdin) can only be given once`)
		}
		read := 0
		sc := bufio.NewScanner(stdin)
		sc.Buffer(make([]byte, 0, 64*1024), 1024*1024) // magnets with many trackers get long
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			out = append(out, line)
			read++
		}
		if err := sc.Err(); err != nil {
			return nil, fmt.Errorf("get -: reading stdin: %w", err)
		}
		if read == 0 {
			return nil, errors.New("get -: no targets on stdin; pipe in page URLs or magnet links, one per line")
		}
	}
	return out, nil
}

// sendToQbit submits the targets and prints a line per torrent as it goes.
// It keeps going past individual failures and returns an error counting them.
func sendToQbit(s *core.Service, targets []string) error {
	if qbitURL == "" {
		return errNoQbit
	}
	opts := core.AddOptions{SavePath: qbitSavePath, Category: qbitCategory, Paused: qbitPaused}
	outs, err := s.Submit(targets, opts, func(o core.Outcome) {
		if o.Err != nil {
			fmt.Fprintf(os.Stderr, "✗ %s: %v\n", o.Target, o.Err)
		} else {
			fmt.Printf("✓ sent to qBittorrent: %s\n", o.Name)
		}
	})
	if err != nil {
		return err
	}
	if failed := core.Failed(outs); failed > 0 {
		return fmt.Errorf("%d of %d torrent(s) failed", failed, len(targets))
	}
	return nil
}

func init() {
	rootCmd.AddCommand(getCmd)

	f := rootCmd.PersistentFlags()
	f.StringVar(&qbitURL, "qbit-url", os.Getenv("QBIT_URL"), "qBittorrent Web UI URL, e.g. http://nas.local:8080 (env QBIT_URL)")
	f.StringVar(&qbitUser, "qbit-user", os.Getenv("QBIT_USER"), "qBittorrent username (env QBIT_USER)")
	f.StringVar(&qbitPass, "qbit-pass", os.Getenv("QBIT_PASS"), "qBittorrent password; prefer the env var to keep it out of shell history (env QBIT_PASS)")
	f.StringVar(&qbitSavePath, "save-path", os.Getenv("QBIT_SAVE_PATH"), "download directory on the qBittorrent server (env QBIT_SAVE_PATH; default: the category's or server's path)")
	f.StringVarP(&qbitCategory, "category", "c", os.Getenv("QBIT_CATEGORY"), "qBittorrent category, e.g. movies or tv; its save path on the server decides the Jellyfin library (env QBIT_CATEGORY)")
	f.BoolVar(&qbitPaused, "paused", false, "add the torrent without starting it")
}
