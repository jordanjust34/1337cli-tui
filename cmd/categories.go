package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"

	"1337cli/internal/qbit"

	"github.com/spf13/cobra"
)

var categoriesCmd = &cobra.Command{
	Use:   "categories",
	Short: "List qBittorrent categories and where they save",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if qbitURL == "" {
			return errNoQbit
		}
		cats, err := qbit.NewClient(qbitURL, qbitUser, qbitPass).Categories()
		if err != nil {
			return err
		}
		if jsonOut {
			return json.NewEncoder(os.Stdout).Encode(cats)
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "CATEGORY\tSAVE PATH")
		for _, c := range cats {
			fmt.Fprintf(w, "%s\t%s\n", c.Name, c.SavePath)
		}
		return w.Flush()
	},
}

func init() {
	rootCmd.AddCommand(categoriesCmd)
	categoriesCmd.Flags().BoolVar(&jsonOut, "json", false, "print categories as JSON")
}
