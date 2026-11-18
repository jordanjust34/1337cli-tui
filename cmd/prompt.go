package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"1337cli/internal/core"

	"github.com/charmbracelet/x/term"
)

// stdinIsTerminal reports whether stdin is interactive. A variable so tests can
// run --interactive without a real terminal.
// A character-device check isn't enough: /dev/null is one too.
var stdinIsTerminal = func() bool { return term.IsTerminal(os.Stdin.Fd()) }

// pickTargets asks which results to send and, if needCategory is set, which
// qBittorrent category to use. Prompts and errors go to out (stderr in the CLI)
// so stdout keeps only the table. Invalid input asks again; an empty answer or
// EOF at the first prompt, or EOF at the category prompt, returns no targets.
// An empty category means the server default. If cats fails, it warns and
// carries on without a category rather than losing the selection.
func pickTargets(in io.Reader, out io.Writer, results []core.Result, cats func() ([]core.Category, error), needCategory bool) (targets []string, category string, err error) {
	r := bufio.NewReader(in) // one reader, so buffered input isn't lost between prompts

	var picks []int
	for {
		line, ok, err := ask(r, out, "Send which? (e.g. 1,3 or 2-5, enter to quit): ")
		if err != nil || !ok || line == "" {
			return nil, "", err
		}
		if picks, err = parseSelection([]string{line}, len(results)); err == nil {
			break
		}
		// parseSelection's errors name the flag; here the user typed at a prompt.
		fmt.Fprintln(out, strings.TrimPrefix(err.Error(), "--get "))
	}
	for _, i := range picks {
		targets = append(targets, results[i-1].URL)
	}
	if !needCategory {
		return targets, "", nil
	}

	list, err := cats()
	if err != nil {
		fmt.Fprintf(out, "warning: can't list categories (%v); sending without one\n", err)
		return targets, "", nil
	}
	if len(list) == 0 {
		return targets, "", nil
	}
	fmt.Fprintln(out)
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	for i, c := range list {
		fmt.Fprintf(w, "%2d\t%s\t%s\n", i+1, c.Name, c.SavePath)
	}
	w.Flush()
	for {
		line, ok, err := ask(r, out, "Category? (number, enter for server default): ")
		if err != nil || !ok {
			return nil, "", err
		}
		if line == "" {
			return targets, "", nil
		}
		if i, err := strconv.Atoi(line); err == nil && i >= 1 && i <= len(list) {
			return targets, list[i-1].Name, nil
		}
		fmt.Fprintf(out, "%q: enter a number from 1 to %d, or press enter for the server default\n", line, len(list))
	}
}

// ask prints prompt and reads one trimmed line. ok is false at EOF with
// nothing typed, so Ctrl-D quits instead of submitting a half-typed answer.
func ask(r *bufio.Reader, out io.Writer, prompt string) (line string, ok bool, err error) {
	fmt.Fprint(out, prompt)
	line, err = r.ReadString('\n')
	if errors.Is(err, io.EOF) {
		fmt.Fprintln(out) // the user's Ctrl-D left the cursor on the prompt line
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("reading answer: %w", err)
	}
	return strings.TrimSpace(line), true, nil
}
