package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/vlle/jig/internal/anim"
)

var logoGlyphs = [][]string{
	{
		"     ██╗",
		"     ██║",
		"     ██║",
		"██   ██║",
		"╚█████╔╝",
		" ╚════╝ ",
	},
	{
		"██╗",
		"██║",
		"██║",
		"██║",
		"██║",
		"╚═╝",
	},
	{
		" ██████╗ ",
		"██╔════╝ ",
		"██║  ███╗",
		"██║   ██║",
		"╚██████╔╝",
		" ╚═════╝ ",
	},
}

var (
	logoColors  = []lipgloss.Color{"#22D3EE", "#818CF8", "#C084FC"}
	logoShadow  = lipgloss.NewStyle().Foreground(lipgloss.Color("#475569"))
	homeAccent  = lipgloss.NewStyle().Foreground(lipgloss.Color("#818CF8"))
	homeOK      = lipgloss.NewStyle().Foreground(lipgloss.Color("78"))
	homeWarn    = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	homeKey     = lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Bold(true)
	hopCycle    = 32
	hopDuration = 3
	hopStagger  = 4
)

func letterHop(letter, tick int) int {
	phase := ((tick-letter*hopStagger)%hopCycle + hopCycle) % hopCycle
	if phase < hopDuration {
		return 1
	}
	return 0
}

func renderLogo(tick int) string {
	height := len(logoGlyphs[0]) + 1
	rows := make([]strings.Builder, height)

	for letter, glyph := range logoGlyphs {
		width := len([]rune(glyph[0]))
		top := 1 - letterHop(letter, tick)
		block := lipgloss.NewStyle().Foreground(logoColors[letter]).Bold(true)

		for row := range height {
			if letter > 0 {
				rows[row].WriteString(" ")
			}
			index := row - top
			if index < 0 || index >= len(glyph) {
				rows[row].WriteString(strings.Repeat(" ", width))
				continue
			}
			rows[row].WriteString(paintGlyph(glyph[index], block))
		}
	}

	lines := make([]string, height)
	for i := range rows {
		lines[i] = rows[i].String()
	}
	return strings.Join(lines, "\n")
}

func paintGlyph(line string, block lipgloss.Style) string {
	var out, run strings.Builder
	solid := false
	flush := func() {
		if run.Len() == 0 {
			return
		}
		if solid {
			out.WriteString(block.Render(run.String()))
		} else {
			out.WriteString(logoShadow.Render(run.String()))
		}
		run.Reset()
	}

	for _, r := range line {
		isSolid := r == '█'
		if r != ' ' && isSolid != solid {
			flush()
			solid = isSolid
		}
		run.WriteRune(r)
	}
	flush()
	return out.String()
}

type recentRun struct {
	tool string
	at   time.Time
	code int
}

func recentRuns(limit int) []recentRun {
	path := runlogPath()
	if path == "" || path == "-" {
		return nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = file.Close() }()

	const window = 128 << 10
	if stat, statErr := file.Stat(); statErr == nil && stat.Size() > window {
		_, _ = file.Seek(stat.Size()-window, io.SeekStart)
	}
	raw, err := io.ReadAll(file)
	if err != nil {
		return nil
	}

	var lines [][]byte
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for scanner.Scan() {
		lines = append(lines, append([]byte(nil), scanner.Bytes()...))
	}

	var runs []recentRun
	seen := map[string]bool{}
	for i := len(lines) - 1; i >= 0 && len(runs) < limit; i-- {
		var record struct {
			TS       string `json:"ts"`
			Source   string `json:"source"`
			Cmd      string `json:"cmd"`
			Tool     string `json:"tool"`
			ExitCode int    `json:"exit_code"`
		}
		if json.Unmarshal(lines[i], &record) != nil || record.Tool == "" || seen[record.Tool] {
			continue
		}
		if record.Cmd != "run" && record.Source != "tool" {
			continue
		}
		at, parseErr := time.Parse(time.RFC3339, record.TS)
		if parseErr != nil {
			continue
		}
		seen[record.Tool] = true
		runs = append(runs, recentRun{tool: record.Tool, at: at, code: record.ExitCode})
	}
	return runs
}

func ago(at time.Time) string {
	elapsed := time.Since(at)
	switch {
	case elapsed < time.Minute:
		return "just now"
	case elapsed < time.Hour:
		return fmt.Sprintf("%dm ago", int(elapsed.Minutes()))
	case elapsed < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(elapsed.Hours()))
	}
	return fmt.Sprintf("%dd ago", int(elapsed.Hours()/24))
}

func (m tuiModel) homeView() string {
	var counts [3]int
	for _, tool := range m.all {
		switch {
		case tool.Status == "deprecated":
			counts[2]++
		case tool.Kind == "env":
			counts[1]++
		default:
			counts[0]++
		}
	}

	stats := plural(counts[0], "tool")
	if counts[1] > 0 {
		stats += " · " + plural(counts[1], "secret profile")
	}
	if counts[2] > 0 {
		stats += fmt.Sprintf(" · %d deprecated", counts[2])
	}

	sections := []string{
		renderLogo(m.tick),
		"",
		styleDim.Render("find · run · scaffold the scripts in ") + homeAccent.Render(filepath.Base(m.cfg.Root)),
		"",
		stats,
		m.doctorLine(),
	}

	if len(m.all) == 0 {
		sections = append(sections, "", homeWarn.Render("the registry is empty — `jig new <id>`, or register what doctor finds"))
	}

	if len(m.recent) > 0 {
		sections = append(sections, "", styleDim.Render("recent"))
		width := 0
		for _, run := range m.recent {
			width = max(width, len(run.tool))
		}
		for _, run := range m.recent {
			mark := homeOK.Render("✓")
			if run.code != 0 {
				mark = styleDanger.Render("✗")
			}
			sections = append(sections, fmt.Sprintf("%s %s  %s", mark, pad(run.tool, width), styleDim.Render(pad(ago(run.at), 9))))
		}
	}

	keys := []string{
		homeKey.Render("type") + styleDim.Render(" to search"),
		homeKey.Render("enter") + styleDim.Render(" browse"),
		homeKey.Render("ctrl+d") + styleDim.Render(" doctor"),
		homeKey.Render("esc") + styleDim.Render(" quit"),
	}
	sections = append(sections, "", strings.Join(keys, styleDim.Render("  ·  ")))
	if m.status != "" {
		sections = append(sections, "", homeWarn.Render(m.status))
	}

	block := lipgloss.JoinVertical(lipgloss.Center, sections...)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, block)
}

func (m tuiModel) doctorLine() string {
	switch {
	case m.doctorRunning:
		frame := homeAccent.Render(anim.Frames[m.tick%len(anim.Frames)])
		return fmt.Sprintf("%s %s", frame, styleDim.Render(fmt.Sprintf("doctor is scanning · %d files", m.doctorFiles.Load())))
	case m.doctor == nil:
		return ""
	case m.doctor.problems() == 0:
		return homeOK.Render("✓ doctor: clean")
	}
	return homeWarn.Render(fmt.Sprintf("! doctor: %d problems — ctrl+d", m.doctor.problems()))
}

func plural(count int, noun string) string {
	if count == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", count, noun)
}
