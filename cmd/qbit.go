package cmd

import (
	"errors"
	"fmt"
	"os"

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
	Use:   "get <torrent-page-url|magnet>...",
	Short: "Send 1337x torrents to qBittorrent and start downloading",
	Example: `  1337cli get https://1337x.to/torrent/1699436/Ubuntu-16-04-1-LTS-Desktop-64-bit/
  1337cli get 'magnet:?xt=urn:btih:...' --save-path /downloads/linux`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return sendToQbit(newService(), args)
	},
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
