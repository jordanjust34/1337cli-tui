package main

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type screen int

const (
	screenSearch screen = iota
	screenSearching
	screenResults
	screenCategory
	screenConfirm
)

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	cursorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	errStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	cmdStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("229")).Border(lipgloss.RoundedBorder()).Padding(0, 1)
)

type searchDoneMsg struct {
	seq     int
	results []result
	err     error
}

type categoriesMsg struct {
	cats []category
	err  error
}

type model struct {
	cli   string
	limit int

	screen  screen
	input   textinput.Model
	spinner spinner.Model
	err     error
	width   int
	height  int

	query    string
	seq      int // identifies the latest search, so stale results are dropped
	results  []result
	selected map[int]bool
	cursor   int
	offset   int

	// cats[0] is always "no category" (use the CLI's default from its config).
	cats     []category
	catsErr  error
	catIndex int
	paused   bool

	// Set when the user confirms; main runs it after the TUI exits.
	runArgs []string
}

func newModel(cli string, limit int) model {
	ti := textinput.New()
	ti.Placeholder = "e.g. superman 2025"
	ti.Prompt = "🔎 "
	ti.Focus()

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	return model{
		cli:      cli,
		limit:    limit,
		input:    ti,
		spinner:  sp,
		selected: map[int]bool{},
		cats:     []category{{Name: ""}},
	}
}

func (m model) Init() tea.Cmd {
	// Load categories in the background while the user types a query.
	return tea.Batch(textinput.Blink, m.loadCategories())
}

func (m model) loadCategories() tea.Cmd {
	cli := m.cli
	return func() tea.Msg {
		var cats []category
		err := runJSON(context.Background(), cli, []string{"categories", "--json"}, &cats)
		return categoriesMsg{cats, err}
	}
}

func (m model) search(query string) tea.Cmd {
	cli, limit, seq := m.cli, m.limit, m.seq
	return func() tea.Msg {
		var rs []result
		err := runJSON(context.Background(), cli, searchArgs(query, limit), &rs)
		return searchDoneMsg{seq, rs, err}
	}
}

// chosen returns the selected results in list order, or the one under the
// cursor if nothing is explicitly selected.
func (m model) chosen() []result {
	var out []result
	for i, r := range m.results {
		if m.selected[i] {
			out = append(out, r)
		}
	}
	if len(out) == 0 && len(m.results) > 0 {
		out = append(out, m.results[m.cursor])
	}
	return out
}

func (m model) category() string { return m.cats[m.catIndex].Name }

func (m model) commandArgs() []string { return getArgs(m.chosen(), m.category(), m.paused) }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

	case categoriesMsg:
		m.catsErr = msg.err
		if msg.err == nil {
			m.cats = append([]category{{Name: ""}}, msg.cats...)
		}
		return m, nil

	case searchDoneMsg:
		if m.screen != screenSearching || msg.seq != m.seq {
			return m, nil // cancelled or superseded by a newer search
		}
		if msg.err != nil {
			m.err = msg.err
			m.screen = screenSearch
			return m, textinput.Blink
		}
		m.results, m.selected, m.cursor, m.offset = msg.results, map[int]bool{}, 0, 0
		m.screen = screenResults
		return m, nil

	case spinner.TickMsg:
		if m.screen != screenSearching {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	switch m.screen {
	case screenSearch:
		return m.updateSearch(msg)
	case screenSearching:
		if k, ok := msg.(tea.KeyMsg); ok && k.String() == "esc" {
			m.screen = screenSearch
		}
		return m, nil
	case screenResults:
		return m.updateResults(msg)
	case screenCategory:
		return m.updateCategory(msg)
	case screenConfirm:
		return m.updateConfirm(msg)
	}
	return m, nil
}

func (m model) updateSearch(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "esc":
			if len(m.results) > 0 {
				m.screen = screenResults
				return m, nil
			}
			return m, tea.Quit
		case "enter":
			q := strings.TrimSpace(m.input.Value())
			if q == "" {
				return m, nil
			}
			m.query, m.err = q, nil
			m.seq++
			m.screen = screenSearching
			return m, tea.Batch(m.spinner.Tick, m.search(q))
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m model) updateResults(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch k.String() {
	case "q":
		return m, tea.Quit
	case "esc", "/":
		m.screen = screenSearch
		m.input.Focus()
		return m, textinput.Blink
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.results)-1 {
			m.cursor++
		}
	case " ", "x":
		if len(m.results) > 0 {
			m.selected[m.cursor] = !m.selected[m.cursor]
			if m.cursor < len(m.results)-1 {
				m.cursor++
			}
		}
	case "a":
		all := len(m.chosenExplicit()) < len(m.results)
		for i := range m.results {
			m.selected[i] = all
		}
	case "enter":
		if len(m.results) > 0 {
			m.screen = screenCategory
		}
	}
	m.scroll()
	return m, nil
}

func (m model) chosenExplicit() []int {
	var idx []int
	for i := range m.results {
		if m.selected[i] {
			idx = append(idx, i)
		}
	}
	return idx
}

// listHeight is how many result rows fit on screen.
func (m model) listHeight() int {
	h := m.height - 8 // title, header, blank lines, help
	if h < 3 {
		h = 10
	}
	return h
}

func (m *model) scroll() {
	h := m.listHeight()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+h {
		m.offset = m.cursor - h + 1
	}
}

func (m model) updateCategory(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch k.String() {
	case "esc":
		m.screen = screenResults
	case "up", "k":
		if m.catIndex > 0 {
			m.catIndex--
		}
	case "down", "j":
		if m.catIndex < len(m.cats)-1 {
			m.catIndex++
		}
	case "enter":
		m.screen = screenConfirm
	}
	return m, nil
}

func (m model) updateConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch k.String() {
	case "esc":
		m.screen = screenCategory
	case "p":
		m.paused = !m.paused
	case "enter", "y":
		m.runArgs = m.commandArgs()
		return m, tea.Quit
	case "q":
		return m, tea.Quit
	}
	return m, nil
}

func (m model) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("1337tui") + dimStyle.Render("  search 1337x → send to qBittorrent") + "\n\n")

	switch m.screen {
	case screenSearch:
		b.WriteString("Search 1337x:\n\n" + m.input.View() + "\n")
		if m.err != nil {
			b.WriteString("\n" + errStyle.Render(m.err.Error()) + "\n")
		}
		help := "enter search • ctrl+c quit"
		if len(m.results) > 0 {
			help = "enter search • esc back to results • ctrl+c quit"
		}
		b.WriteString("\n" + dimStyle.Render(help))

	case screenSearching:
		fmt.Fprintf(&b, "%s Searching for %q… (this can take ~15s per page)\n\n", m.spinner.View(), m.query)
		b.WriteString(dimStyle.Render("esc cancel"))

	case screenResults:
		b.WriteString(m.viewResults())

	case screenCategory:
		b.WriteString(m.viewCategory())

	case screenConfirm:
		b.WriteString(m.viewConfirm())
	}
	return b.String()
}

func (m model) viewResults() string {
	var b strings.Builder
	if len(m.results) == 0 {
		fmt.Fprintf(&b, "No results for %q.\n\n", m.query)
		b.WriteString(dimStyle.Render("/ new search • q quit"))
		return b.String()
	}

	nameW := m.width - 40
	if nameW < 20 {
		nameW = 50
	}
	fmt.Fprintf(&b, "Results for %q — %d selected\n\n", m.query, len(m.chosenExplicit()))
	b.WriteString(dimStyle.Render(fmt.Sprintf("      %-*s %6s %5s %10s", nameW, "NAME", "SE", "LE", "SIZE")) + "\n")

	end := min(m.offset+m.listHeight(), len(m.results))
	for i := m.offset; i < end; i++ {
		r := m.results[i]
		box := "[ ]"
		if m.selected[i] {
			box = selectedStyle.Render("[x]")
		}
		line := fmt.Sprintf("%-*s %6d %5d %10s", nameW, truncate(r.Name, nameW), r.Seeders, r.Leechers, r.Size)
		switch {
		case i == m.cursor:
			b.WriteString(cursorStyle.Render("> ") + box + " " + cursorStyle.Render(line))
		case m.selected[i]:
			b.WriteString("  " + box + " " + selectedStyle.Render(line))
		default:
			b.WriteString("  " + box + " " + line)
		}
		b.WriteString("\n")
	}
	if len(m.results) > m.listHeight() {
		b.WriteString(dimStyle.Render(fmt.Sprintf("  %d–%d of %d", m.offset+1, end, len(m.results))) + "\n")
	}
	b.WriteString("\n" + dimStyle.Render("↑/↓ move • space select • a all • enter continue • / new search • q quit"))
	return b.String()
}

func (m model) viewCategory() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Send %d torrent(s) to which qBittorrent category?\n\n", len(m.chosen()))
	nameW := 0
	for _, c := range m.cats {
		nameW = max(nameW, len([]rune(c.Name)))
	}
	for i, c := range m.cats {
		label := c.Name
		if label == "" {
			label = "(none — use the CLI's default)"
		}
		if c.SavePath != "" {
			label = fmt.Sprintf("%-*s", nameW, label) + dimStyle.Render("  → "+c.SavePath)
		}
		if i == m.catIndex {
			b.WriteString(cursorStyle.Render("> ") + cursorStyle.Render(label) + "\n")
		} else {
			b.WriteString("  " + label + "\n")
		}
	}
	if m.catsErr != nil {
		b.WriteString("\n" + errStyle.Render("couldn't load categories: "+firstLine(m.catsErr.Error())) + "\n")
	}
	b.WriteString("\n" + dimStyle.Render("↑/↓ move • enter continue • esc back"))
	return b.String()
}

func (m model) viewConfirm() string {
	var b strings.Builder
	b.WriteString("About to send:\n\n")
	for _, r := range m.chosen() {
		b.WriteString("  • " + truncate(r.Name, max(m.width-6, 40)) + dimStyle.Render("  "+r.Size) + "\n")
	}
	paused := "start downloading"
	if m.paused {
		paused = "add paused"
	}
	fmt.Fprintf(&b, "\nCategory: %s   Mode: %s\n\n", orNone(m.category()), paused)
	b.WriteString("Command:\n" + cmdStyle.Width(max(m.width-4, 40)).Render(shellJoin(m.cli, m.commandArgs())) + "\n\n")
	b.WriteString(dimStyle.Render("enter run • p toggle paused • esc back • q quit without running"))
	return b.String()
}

func orNone(s string) string {
	if s == "" {
		return "(default)"
	}
	return s
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
