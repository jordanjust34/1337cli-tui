package main

import (
	"fmt"
	"strings"

	"1337cli/internal/core"

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
	screenSubmitting
	screenDone
)

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	cursorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("212")).Bold(true)
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	dimStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	errStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
)

// backend is the shared library the TUI drives; *core.Service implements it,
// and tests substitute a recording fake.
type backend interface {
	Search(query string, page, limit int) ([]core.Result, error)
	Categories() ([]core.Category, error)
	Submit(targets []string, opts core.AddOptions, progress func(core.Outcome)) ([]core.Outcome, error)
	Ping() (string, error)
}

type searchDoneMsg struct {
	seq     int
	results []core.Result
	err     error
}

type categoriesMsg struct {
	cats []core.Category
	err  error
}

type pingMsg struct {
	version string
	err     error
}

type submitDoneMsg struct {
	outcomes []core.Outcome
	err      error
}

type model struct {
	svc   backend
	limit int
	// defaults holds the configured category and save path (QBIT_CATEGORY,
	// QBIT_SAVE_PATH), used when the user picks "no category".
	defaults core.AddOptions

	screen  screen
	input   textinput.Model
	spinner spinner.Model
	err     error
	width   int
	height  int

	query    string
	seq      int // identifies the latest search, so stale results are dropped
	results  []core.Result
	selected map[int]bool
	cursor   int
	offset   int

	// cats[0] is always "no category" (use the configured default).
	cats     []core.Category
	catsErr  error
	catIndex int
	paused   bool

	// Startup connectivity check; shown on the search screen but never
	// blocks searching, since 1337x works without qBittorrent.
	pinged      bool
	qbitVersion string
	pingErr     error

	// Result of the last confirmed submission.
	outcomes  []core.Outcome
	submitErr error
	// failed is set once any submission fails, so main can exit non-zero.
	failed bool
}

func newModel(svc backend, defaults core.AddOptions, limit int) model {
	ti := textinput.New()
	ti.Placeholder = "e.g. superman 2025"
	ti.Prompt = "🔎 "
	ti.Focus()

	sp := spinner.New()
	sp.Spinner = spinner.Dot

	return model{
		svc:      svc,
		defaults: defaults,
		limit:    limit,
		input:    ti,
		spinner:  sp,
		selected: map[int]bool{},
		cats:     []core.Category{{Name: ""}},
	}
}

func (m model) Init() tea.Cmd {
	// Load categories and check qBittorrent in the background while the
	// user types a query, so a bad URL or password shows up before sending.
	return tea.Batch(textinput.Blink, m.loadCategories(), m.ping())
}

func (m model) ping() tea.Cmd {
	svc := m.svc
	return func() tea.Msg {
		v, err := svc.Ping()
		return pingMsg{v, err}
	}
}

func (m model) loadCategories() tea.Cmd {
	svc := m.svc
	return func() tea.Msg {
		cats, err := svc.Categories()
		return categoriesMsg{cats, err}
	}
}

func (m model) search(query string) tea.Cmd {
	svc, limit, seq := m.svc, m.limit, m.seq
	return func() tea.Msg {
		rs, err := svc.Search(query, 1, limit)
		return searchDoneMsg{seq, rs, err}
	}
}

func (m model) submit() tea.Cmd {
	svc, opts := m.svc, m.addOptions()
	var targets []string
	for _, r := range m.chosen() {
		targets = append(targets, r.URL)
	}
	return func() tea.Msg {
		outs, err := svc.Submit(targets, opts, nil)
		return submitDoneMsg{outs, err}
	}
}

// chosen returns the selected results in list order, or the one under the
// cursor if nothing is explicitly selected.
func (m model) chosen() []core.Result {
	var out []core.Result
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

// addOptions is where and how the chosen torrents will be added: the picked
// category, else the configured default, plus any configured save path.
func (m model) addOptions() core.AddOptions {
	opts := m.defaults
	if c := m.category(); c != "" {
		opts.Category = c
	}
	opts.Paused = m.paused
	return opts
}

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
			m.cats = append([]core.Category{{Name: ""}}, msg.cats...)
		}
		return m, nil

	case pingMsg:
		m.pinged, m.qbitVersion, m.pingErr = true, msg.version, msg.err
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

	case submitDoneMsg:
		m.outcomes, m.submitErr = msg.outcomes, msg.err
		if msg.err != nil || core.Failed(msg.outcomes) > 0 {
			m.failed = true
		} else {
			m.selected = map[int]bool{}
		}
		m.screen = screenDone
		return m, nil

	case spinner.TickMsg:
		if m.screen != screenSearching && m.screen != screenSubmitting {
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
	case screenDone:
		return m.updateDone(msg)
	}
	return m, nil // screenSubmitting only answers ctrl+c
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
	case "b":
		m.selectBest()
	case "enter":
		if len(m.results) > 0 {
			m.screen = screenCategory
		}
	}
	m.scroll()
	return m, nil
}

// selectBest makes the top-seeded result the only selection and moves the
// cursor to it.
func (m *model) selectBest() {
	best := core.Best(m.results, 1)
	if len(best) == 0 {
		return
	}
	for i, r := range m.results {
		if r == best[0] {
			m.selected = map[int]bool{i: true}
			m.cursor = i
			return
		}
	}
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
		m.screen = screenSubmitting
		return m, tea.Batch(m.spinner.Tick, m.submit())
	case "q":
		return m, tea.Quit
	}
	return m, nil
}

func (m model) updateDone(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch k.String() {
	case "esc":
		m.screen = screenResults
	case "enter", "/":
		m.screen = screenSearch
		m.input.Focus()
		return m, textinput.Blink
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
		switch {
		case m.pingErr != nil:
			b.WriteString("\n" + errStyle.Render("qBittorrent unreachable: "+firstLine(m.pingErr.Error())) + "\n")
		case m.pinged:
			b.WriteString("\n" + dimStyle.Render("connected to qBittorrent "+m.qbitVersion) + "\n")
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

	case screenSubmitting:
		fmt.Fprintf(&b, "%s Sending %d torrent(s) to qBittorrent…\n\n", m.spinner.View(), len(m.chosen()))
		b.WriteString(dimStyle.Render("ctrl+c quit (torrents already sent stay queued)"))

	case screenDone:
		b.WriteString(m.viewDone())
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
	b.WriteString("\n" + dimStyle.Render("↑/↓ move • space select • a all • b best • enter continue • / new search • q quit"))
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
			label = "(none — " + m.defaultCategory() + ")"
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
	opts := m.addOptions()
	category := opts.Category
	if category == "" {
		category = "(none — server default)"
	} else if p := m.cats[m.catIndex].SavePath; p != "" && opts.SavePath == "" {
		category += dimStyle.Render("  → " + p)
	}
	fmt.Fprintf(&b, "\nCategory: %s\n", category)
	if opts.SavePath != "" {
		fmt.Fprintf(&b, "Save path: %s %s\n", opts.SavePath, dimStyle.Render("(QBIT_SAVE_PATH, overrides the category's path)"))
	}
	fmt.Fprintf(&b, "Mode: %s\n\n", paused)
	b.WriteString(dimStyle.Render("enter send • p toggle paused • esc back • q quit without sending"))
	return b.String()
}

// defaultCategory describes what "no category" means with the current config.
func (m model) defaultCategory() string {
	if m.defaults.Category != "" {
		return "use the default, " + m.defaults.Category
	}
	return "use the server's default folder"
}

func (m model) viewDone() string {
	var b strings.Builder
	if m.submitErr != nil {
		b.WriteString(errStyle.Render("Nothing sent: "+m.submitErr.Error()) + "\n")
	} else {
		for _, o := range m.outcomes {
			if o.Err != nil {
				b.WriteString(errStyle.Render("✗ "+o.Target+": "+o.Err.Error()) + "\n")
			} else {
				b.WriteString(selectedStyle.Render("✓") + " sent to qBittorrent: " + o.Name + "\n")
			}
		}
		if failed := core.Failed(m.outcomes); failed > 0 {
			fmt.Fprintf(&b, "\n%d of %d torrent(s) failed.\n", failed, len(m.outcomes))
		}
	}
	b.WriteString("\n" + dimStyle.Render("enter new search • esc back to results • q quit"))
	return b.String()
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
