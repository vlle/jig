package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/vlle/jig/internal/anim"
)

type viewMode int

const (
	viewHome viewMode = iota
	viewBrowse
)

type paneMode int

const (
	paneDoc paneMode = iota
	paneSource
	paneDoctor
)

type tuiModel struct {
	cfg      Config
	all      []Tool
	shown    []Tool
	cursor   int
	offset   int
	view     viewMode
	pane     paneMode
	filter   textinput.Model
	filterOn bool
	preview  viewport.Model
	width    int
	height   int
	chosen   *Tool
	status   string
	tick     int
	ticking  bool
	recent   []recentRun

	doctor        *doctorReport
	doctorRunning bool
	doctorFiles   *atomic.Int64

	sourceID      string
	sourceBody    string
	sourceLoading bool
}

type (
	tickMsg    struct{}
	doctorDone struct{ report doctorReport }
	sourceDone struct {
		id   string
		path string
		body string
		err  error
	}
	editorDone struct{ err error }
)

var (
	styleTitle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#818CF8"))
	styleSelected = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231")).Background(lipgloss.Color("#4338CA"))
	styleDim      = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	styleReadOnly = lipgloss.NewStyle().Foreground(lipgloss.Color("78"))
	styleWrites   = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	styleDanger   = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	styleBorder   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240"))
)

func newTUI(cfg Config, tools []Tool) tuiModel {
	filter := textinput.New()
	filter.Prompt = "/ "
	filter.Placeholder = "search id, summary, tags"

	return tuiModel{
		cfg:         cfg,
		all:         tools,
		shown:       tools,
		filter:      filter,
		preview:     viewport.New(0, 0),
		recent:      recentRuns(3),
		doctorFiles: &atomic.Int64{},
	}
}

func (m tuiModel) Init() tea.Cmd {
	return tea.Batch(tick(), m.runDoctor())
}

func tick() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(time.Time) tea.Msg { return tickMsg{} })
}

func (m tuiModel) runDoctor() tea.Cmd {
	cfg, files := m.cfg, m.doctorFiles
	return func() tea.Msg {
		return doctorDone{report: diagnose(cfg, func(n int) { files.Store(int64(n)) })}
	}
}

func (m *tuiModel) startDoctor() tea.Cmd {
	if m.doctorRunning {
		return nil
	}
	m.doctorRunning = true
	m.doctorFiles.Store(0)
	return tea.Batch(m.runDoctor(), m.ensureTick())
}

func (m tuiModel) animating() bool {
	return m.view == viewHome || m.doctorRunning || m.sourceLoading
}

func (m *tuiModel) ensureTick() tea.Cmd {
	if m.ticking || !m.animating() {
		return nil
	}
	m.ticking = true
	return tick()
}

func (m tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch typed := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = typed.Width, typed.Height
		m.preview.Width = m.paneWidth()
		m.preview.Height = m.bodyHeight()
		return m, m.syncPreview()

	case tickMsg:
		m.tick++
		m.ticking = false
		if m.pane == paneDoctor && m.doctorRunning || m.pane == paneSource && m.sourceLoading {
			m.renderPreview()
		}
		return m, m.ensureTick()

	case doctorDone:
		report := typed.report
		m.doctor, m.doctorRunning = &report, false
		m.renderPreview()
		return m, nil

	case sourceDone:
		if typed.id != m.sourceID {
			return m, nil
		}
		m.sourceLoading = false
		switch {
		case typed.err != nil:
			m.sourceBody = styleDanger.Render(typed.err.Error())
		default:
			m.sourceBody = styleDim.Render(relTo(m.cfg.Root, typed.path)) + "\n\n" + typed.body
		}
		m.renderPreview()
		return m, nil

	case editorDone:
		m.status = ""
		if typed.err != nil {
			m.status = "editor: " + typed.err.Error()
		}
		m.sourceID = ""
		return m, m.syncPreview()

	case tea.KeyMsg:
		if m.view == viewHome {
			return m.updateHome(typed)
		}
		if m.filterOn {
			return m.updateFilter(typed)
		}
		return m.updateBrowse(typed)
	}
	return m, nil
}

func (m tuiModel) updateHome(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.Type {
	case tea.KeyCtrlC, tea.KeyEsc:
		return m, tea.Quit
	case tea.KeyEnter:
		m.view = viewBrowse
		return m, m.syncPreview()
	case tea.KeyCtrlD:
		m.view, m.pane = viewBrowse, paneDoctor
		cmd := m.syncPreview()
		if m.doctor == nil {
			cmd = tea.Batch(cmd, m.startDoctor())
		}
		return m, cmd
	case tea.KeyRunes, tea.KeySpace:
		m.view, m.filterOn = viewBrowse, true
		m.filter.Focus()
		m.filter.SetValue(strings.TrimLeft(string(key.Runes), "/"))
		m.filter.CursorEnd()
		m.applyFilter()
		return m, tea.Batch(textinput.Blink, m.syncPreview())
	}
	return m, nil
}

func (m tuiModel) updateFilter(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		m.filterOn = false
		m.filter.Blur()
		m.filter.SetValue("")
		m.applyFilter()
		return m, m.syncPreview()
	case "enter":
		m.filterOn = false
		m.filter.Blur()
		return m, nil
	case "down", "up", "tab":
		m.filterOn = false
		m.filter.Blur()
		return m.updateBrowse(key)
	}

	var cmd tea.Cmd
	m.filter, cmd = m.filter.Update(key)
	m.applyFilter()
	return m, tea.Batch(cmd, m.syncPreview())
}

func (m tuiModel) updateBrowse(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "esc":
		if m.filter.Value() != "" {
			m.filter.SetValue("")
			m.applyFilter()
			return m, m.syncPreview()
		}
		m.view = viewHome
		m.recent = recentRuns(3)
		return m, m.ensureTick()

	case "/":
		m.filterOn = true
		m.filter.Focus()
		return m, textinput.Blink

	case "j", "down":
		m.move(1)
	case "k", "up":
		m.move(-1)
	case "g", "home":
		m.cursor, m.offset = 0, 0
	case "G", "end":
		m.cursor = max(len(m.shown)-1, 0)
		m.offset = max(m.cursor-m.bodyHeight()+1, 0)

	case "tab":
		switch m.pane {
		case paneDoc:
			m.pane = paneSource
		default:
			m.pane = paneDoc
		}

	case "d":
		m.pane = paneDoctor
		if m.doctor == nil {
			return m, tea.Batch(m.startDoctor(), m.syncPreview())
		}
	case "r":
		if m.pane == paneDoctor {
			return m, tea.Batch(m.startDoctor(), m.syncPreview())
		}

	case "enter":
		if tool := m.current(); tool != nil {
			m.chosen = tool
			return m, tea.Quit
		}

	case "e":
		if tool := m.current(); tool != nil {
			return m, tea.ExecProcess(editorCommand(*tool), func(err error) tea.Msg { return editorDone{err: err} })
		}

	case "y":
		if tool := m.current(); tool != nil {
			line := fmt.Sprintf("cd %s && %s", shellQuote(tool.WorkdirAbs), tool.Run)
			if err := clipboard.WriteAll(line); err != nil {
				m.status = "copy failed: " + err.Error()
			} else {
				m.status = "copied: " + line
			}
		}

	case "pgdown", "ctrl+f":
		m.preview.ScrollDown(m.bodyHeight() / 2)
		return m, nil
	case "pgup", "ctrl+b":
		m.preview.ScrollUp(m.bodyHeight() / 2)
		return m, nil
	}
	return m, m.syncPreview()
}

func editorCommand(tool Tool) *exec.Cmd {
	path := sourcePath(tool)
	if path == "" {
		path = tool.ManifestPath
	}

	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}

	fields := strings.Fields(editor)
	return exec.Command(fields[0], append(fields[1:], path)...)
}

func (m *tuiModel) move(delta int) {
	if len(m.shown) == 0 {
		return
	}
	m.cursor = min(max(m.cursor+delta, 0), len(m.shown)-1)

	visible := m.bodyHeight()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+visible {
		m.offset = m.cursor - visible + 1
	}
}

func (m *tuiModel) applyFilter() {
	query := m.filter.Value()

	m.shown = nil
	for _, tool := range m.all {
		if tool.matches(query) {
			m.shown = append(m.shown, tool)
		}
	}
	m.cursor, m.offset = 0, 0
}

func (m tuiModel) current() *Tool {
	if m.cursor < 0 || m.cursor >= len(m.shown) {
		return nil
	}
	return &m.shown[m.cursor]
}

func (m *tuiModel) syncPreview() tea.Cmd {
	var cmd tea.Cmd
	if tool := m.current(); m.pane == paneSource && tool != nil && tool.ID != m.sourceID {
		cmd = m.loadSource(*tool)
	}
	m.renderPreview()
	m.preview.GotoTop()
	return tea.Batch(cmd, m.ensureTick())
}

func (m *tuiModel) loadSource(tool Tool) tea.Cmd {
	m.sourceID, m.sourceBody, m.sourceLoading = tool.ID, "", true
	width := m.paneWidth()
	return func() tea.Msg {
		path := sourcePath(tool)
		if path == "" {
			return sourceDone{id: tool.ID, err: errors.New("no source found, set `source` in the manifest")}
		}
		body, err := highlight(path, width)
		return sourceDone{id: tool.ID, path: path, body: body, err: err}
	}
}

func (m *tuiModel) renderPreview() {
	frame := styleTitle.Render(anim.Frames[m.tick%len(anim.Frames)])

	switch m.pane {
	case paneDoctor:
		if m.doctorRunning {
			m.preview.SetContent(fmt.Sprintf("%s %s", frame, styleDim.Render(fmt.Sprintf("doctor is scanning · %d files", m.doctorFiles.Load()))))
			return
		}
		if m.doctor != nil {
			var out strings.Builder
			writeDoctor(&out, *m.doctor, fullPalette())
			m.preview.SetContent(lipgloss.NewStyle().Width(m.paneWidth()).Render(out.String() + styleDim.Render("r rerun · tab tool docs")))
		}
		return

	case paneSource:
		if m.sourceLoading {
			m.preview.SetContent(fmt.Sprintf("%s %s", frame, styleDim.Render("loading source")))
			return
		}
		m.preview.SetContent(m.sourceBody)
		return
	}

	tool := m.current()
	if tool == nil {
		m.preview.SetContent(styleDim.Render("nothing matches — `jig new <id>` if this really is new"))
		return
	}
	m.preview.SetContent(lipgloss.NewStyle().Width(m.paneWidth()).Render(m.describe(*tool)))
}

func (m tuiModel) describe(tool Tool) string {
	var out strings.Builder

	fmt.Fprintf(&out, "%s\n%s\n\n", styleTitle.Render(tool.ID), tool.Summary)
	if why := strings.TrimSpace(tool.Why); why != "" {
		fmt.Fprintf(&out, "%s\n\n", why)
	}

	for _, field := range toolFields(m.cfg, tool, tuiSafety(tool.Safety)) {
		fmt.Fprintf(&out, "%s %s\n", styleDim.Render(pad(field[0], 10)), field[1])
	}

	if len(tool.Args) > 0 {
		out.WriteString("\n" + styleTitle.Render("flags") + "\n")
		for _, arg := range tool.Args {
			line := fmt.Sprintf("%s %s", pad(arg.Flag, 12), arg.Desc)
			if arg.Example != "" {
				line += styleDim.Render(" (e.g. " + arg.Example + ")")
			}
			out.WriteString(line + "\n")
		}
	}
	return out.String()
}

func tuiSafety(safety string) string {
	switch safety {
	case "read-only":
		return styleReadOnly.Render(safety)
	case "writes":
		return styleWrites.Render(safety)
	case "destructive":
		return styleDanger.Render(safety)
	}
	return safety
}

func (m tuiModel) listWidth() int {
	return min(max(m.width/3, 24), 42)
}

func (m tuiModel) paneWidth() int {
	return max(m.width-m.listWidth()-5, 20)
}

func (m tuiModel) bodyHeight() int {
	return max(m.height-5, 5)
}

func (m tuiModel) View() string {
	if m.width == 0 {
		return ""
	}
	if m.view == viewHome {
		return m.homeView()
	}

	header := styleTitle.Render("jig") + styleDim.Render(fmt.Sprintf(" · %d of %d", len(m.shown), len(m.all)))
	if m.filterOn || m.filter.Value() != "" {
		header += "   " + m.filter.View()
	}

	listPane := styleBorder.Width(m.listWidth()).Height(m.bodyHeight()).Render(m.renderList())
	previewPane := styleBorder.Width(m.paneWidth()).Height(m.bodyHeight()).Render(m.preview.View())

	pane := map[paneMode]string{paneDoc: "doc", paneSource: "src", paneDoctor: "doctor"}[m.pane]
	hint := styleDim.Render(fmt.Sprintf("j/k · / search · tab %s · enter run · e edit · d doctor · y copy · esc home · q quit", pane))
	if m.status != "" {
		hint = styleReadOnly.Render(m.status)
	}

	return header + "\n" + lipgloss.JoinHorizontal(lipgloss.Top, listPane, previewPane) + "\n" + hint
}

func (m tuiModel) renderList() string {
	if len(m.shown) == 0 {
		return styleDim.Render("empty")
	}

	end := min(m.offset+m.bodyHeight(), len(m.shown))
	width := m.listWidth() - 2

	var out strings.Builder
	for i := m.offset; i < end; i++ {
		tool := m.shown[i]
		label := tool.ID
		if tool.Kind == "env" {
			label = "· " + label
		}
		if len(label) > width {
			label = label[:width]
		}
		if i == m.cursor {
			out.WriteString(styleSelected.Render(pad(label, width)) + "\n")
			continue
		}
		out.WriteString(label + "\n")
	}
	return out.String()
}

func interactive() bool {
	for _, file := range []*os.File{os.Stdin, os.Stdout} {
		stat, err := file.Stat()
		if err != nil || stat.Mode()&os.ModeCharDevice == 0 {
			return false
		}
	}
	return true
}

func runTUI(cfg Config, record *runRecord) error {
	if !interactive() {
		fmt.Fprint(os.Stderr, usage)
		return cmdList(cfg, nil)
	}

	result := scan(cfg)
	var tools []Tool
	for _, tool := range result.Tools {
		if tool.Status != "deprecated" {
			tools = append(tools, tool)
		}
	}

	model := newTUI(cfg, tools)
	model.doctorRunning = true
	if len(result.Errors) > 0 {
		model.status = fmt.Sprintf("%d manifests failed to load, see `jig doctor`", len(result.Errors))
	}

	final, err := tea.NewProgram(model, tea.WithAltScreen()).Run()
	if err != nil {
		return err
	}

	if done, ok := final.(tuiModel); ok && done.chosen != nil {
		return cmdRun(cfg, []string{done.chosen.ID}, record)
	}
	return nil
}
