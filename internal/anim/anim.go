package anim

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"
)

const (
	frameInterval = 80 * time.Millisecond
	esc           = "\033["
	maxPanelRows  = 16
)

var Frames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type Palette struct {
	Reset, Bold, Dim, Red, Green, Yellow, Cyan string
}

var ansiPalette = Palette{
	Reset:  esc + "0m",
	Bold:   esc + "1m",
	Dim:    esc + "2m",
	Red:    esc + "31m",
	Green:  esc + "32m",
	Yellow: esc + "33m",
	Cyan:   esc + "36m",
}

type Options struct {
	Mode    string
	LogPath string
	Tool    string
}

type Screen struct {
	C        Palette
	mu       sync.Mutex
	out      *os.File
	animated bool
	width    int
	drawn    int
	hidden   bool
	signals  chan os.Signal
	log      *os.File
	logPath  string
	started  time.Time
}

type Task struct {
	Name    string
	Detail  string
	Err     error
	Elapsed time.Duration
	state   string
}

func NewScreen(opts Options) (*Screen, error) {
	s := &Screen{out: os.Stderr, started: time.Now()}

	switch opts.Mode {
	case "", "auto":
		s.animated = Interactive(s.out)
	case "always":
		s.animated = true
	case "never":
	default:
		return nil, fmt.Errorf("unknown animation mode %q, expected auto, always or never", opts.Mode)
	}

	if s.animated {
		s.C = ansiPalette
		s.width = terminalWidth()
	}

	path := opts.LogPath
	if path == "auto" {
		path = DefaultLogPath(opts.Tool)
	}
	if path != "" && path != "-" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("create log dir: %w", err)
		}
		file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return nil, fmt.Errorf("open log %s: %w", path, err)
		}
		s.log, s.logPath = file, path
		s.writeLog(fmt.Sprintf("=== %s ===", opts.Tool))
	}

	return s, nil
}

func Interactive(file *os.File) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if term := os.Getenv("TERM"); term == "" || term == "dumb" {
		return false
	}
	stat, err := file.Stat()
	return err == nil && stat.Mode()&os.ModeCharDevice != 0
}

func DefaultLogPath(tool string) string {
	base := os.Getenv("XDG_STATE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return filepath.Join(os.TempDir(), tool+".log")
		}
		base = filepath.Join(home, ".local", "state")
	}
	stamp := time.Now().Format("20060102-150405")
	return filepath.Join(base, "jig", "logs", fmt.Sprintf("%s-%s.log", tool, stamp))
}

func terminalWidth() int {
	if tty, err := os.Open("/dev/tty"); err == nil {
		defer func() { _ = tty.Close() }()
		cmd := exec.Command("stty", "size")
		cmd.Stdin = tty
		if out, runErr := cmd.Output(); runErr == nil {
			if fields := strings.Fields(string(out)); len(fields) == 2 {
				if width, convErr := strconv.Atoi(fields[1]); convErr == nil && width > 0 {
					return width
				}
			}
		}
	}
	if width, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && width > 0 {
		return width
	}
	return 0
}

func (s *Screen) Animated() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.animated
}

func (s *Screen) Note(format string, args ...any) {
	s.mu.Lock()
	defer s.mu.Unlock()

	line := fmt.Sprintf(format, args...)
	s.writeLog(line)
	s.drawn = 0
	_, _ = fmt.Fprintln(s.out, line)
}

func (s *Screen) Event(format string, args ...any) {
	s.mu.Lock()
	defer s.mu.Unlock()

	line := fmt.Sprintf(format, args...)
	s.writeLog(line)
	if !s.animated {
		_, _ = fmt.Fprintln(s.out, StripANSI(strings.TrimSpace(line)))
	}
}

func (s *Screen) Render(lines []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.draw(lines)
}

func (s *Screen) Commit(lines []string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, line := range lines {
		s.writeLog(line)
	}
	if s.animated {
		s.draw(lines)
		s.drawn = 0
		return
	}
	for _, line := range lines {
		if trimmed := strings.TrimSpace(StripANSI(line)); trimmed != "" {
			_, _ = fmt.Fprintln(s.out, trimmed)
		}
	}
}

func (s *Screen) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.showCursor()
	if s.signals != nil {
		signal.Stop(s.signals)
		close(s.signals)
		s.signals = nil
	}
	if s.log == nil {
		return
	}
	s.writeLog("=== end ===")
	_ = s.log.Close()
	s.log = nil
	_, _ = fmt.Fprintf(s.out, "%saudit log: %s%s\n", s.C.Dim, s.logPath, s.C.Reset)
}

func (s *Screen) draw(lines []string) {
	if !s.animated {
		return
	}
	s.hideCursor()

	if s.drawn > 0 {
		_, _ = fmt.Fprintf(s.out, "%s%dA", esc, s.drawn)
	}
	for _, line := range lines {
		_, _ = fmt.Fprintf(s.out, "\r%s%sK\n", Fit(line, s.width), esc)
	}
	s.drawn = len(lines)
}

func (s *Screen) hideCursor() {
	if s.hidden {
		return
	}
	s.hidden = true
	_, _ = fmt.Fprint(s.out, esc+"?25l")

	if s.signals == nil {
		s.signals = make(chan os.Signal, 1)
		signal.Notify(s.signals, os.Interrupt, syscall.SIGTERM)
		go s.restoreOnSignal(s.signals)
	}
}

func (s *Screen) showCursor() {
	if !s.hidden {
		return
	}
	s.hidden = false
	_, _ = fmt.Fprint(s.out, esc+"?25h")
}

func (s *Screen) restoreOnSignal(signals chan os.Signal) {
	sig, ok := <-signals
	if !ok {
		return
	}

	s.mu.Lock()
	s.showCursor()
	s.animated = false
	s.drawn = 0
	s.mu.Unlock()

	signal.Stop(signals)
	if process, err := os.FindProcess(os.Getpid()); err == nil {
		_ = process.Signal(sig)
	}
}

func (s *Screen) writeLog(line string) {
	if s.log == nil {
		return
	}
	line = StripANSI(strings.TrimSpace(line))
	if line == "" {
		return
	}
	_, _ = fmt.Fprintf(s.log, "%7.3fs  %s\n", time.Since(s.started).Seconds(), line)
}

func (s *Screen) Step(title string, fn func(report func(detail string)) error) error {
	started := time.Now()

	var mu sync.Mutex
	detail := ""
	report := func(value string) {
		mu.Lock()
		detail = value
		mu.Unlock()
	}
	current := func() string {
		mu.Lock()
		defer mu.Unlock()
		return detail
	}

	if !s.Animated() {
		err := fn(report)
		s.Commit([]string{s.stepLine(err, -1, title, current(), time.Since(started))})
		return err
	}

	done := make(chan error, 1)
	go func() { done <- fn(report) }()

	ticker := time.NewTicker(frameInterval)
	defer ticker.Stop()

	for tick := 0; ; tick++ {
		select {
		case err := <-done:
			s.Commit([]string{s.stepLine(err, -1, title, current(), time.Since(started))})
			return err
		case <-ticker.C:
			s.Render([]string{s.stepLine(nil, tick, title, current(), time.Since(started))})
		}
	}
}

func (s *Screen) stepLine(err error, tick int, title, detail string, elapsed time.Duration) string {
	mark := s.C.Green + "✓" + s.C.Reset
	switch {
	case tick >= 0:
		mark = s.C.Cyan + Frames[tick%len(Frames)] + s.C.Reset
	case err != nil:
		mark = s.C.Red + "✗" + s.C.Reset
		detail = s.C.Red + oneLine(err.Error()) + s.C.Reset
	}

	line := fmt.Sprintf("  %s %s", mark, title)
	if detail != "" {
		line += "   " + s.C.Dim + detail + s.C.Reset
	}
	return line + fmt.Sprintf("   %s%.1fs%s", s.C.Dim, elapsed.Seconds(), s.C.Reset)
}

func (s *Screen) Count(title string, total int, fn func(advance func(n int)) error) error {
	started := time.Now()

	var mu sync.Mutex
	done := 0
	advance := func(n int) {
		mu.Lock()
		done = min(done+n, total)
		mu.Unlock()
	}
	progress := func() int {
		mu.Lock()
		defer mu.Unlock()
		return done
	}

	line := func(value int, err error) string {
		text := fmt.Sprintf("  %s %s  %s%d/%d   %.1fs%s",
			title, s.Bar(value, total, 24), s.C.Dim, value, total, time.Since(started).Seconds(), s.C.Reset)
		if err != nil {
			text += "  " + s.C.Red + oneLine(err.Error()) + s.C.Reset
		}
		return text
	}

	if !s.Animated() {
		err := fn(advance)
		s.Commit([]string{line(progress(), err)})
		return err
	}

	result := make(chan error, 1)
	go func() { result <- fn(advance) }()

	ticker := time.NewTicker(frameInterval)
	defer ticker.Stop()

	for {
		select {
		case err := <-result:
			s.Commit([]string{line(progress(), err)})
			return err
		case <-ticker.C:
			s.Render([]string{line(progress(), nil)})
		}
	}
}

func (s *Screen) Fan(ctx context.Context, title string, names []string, limit int, fn func(ctx context.Context, name string, report func(detail string)) (string, error)) []Task {
	tasks := make([]Task, len(names))
	for i, name := range names {
		tasks[i] = Task{Name: name, state: "wait"}
	}
	if limit <= 0 || limit > len(names) {
		limit = len(names)
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	slots := make(chan struct{}, max(limit, 1))
	started := make([]time.Time, len(names))

	for i := range tasks {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			select {
			case slots <- struct{}{}:
			case <-ctx.Done():
				mu.Lock()
				tasks[index].state, tasks[index].Err = "fail", ctx.Err()
				mu.Unlock()
				return
			}
			defer func() { <-slots }()

			mu.Lock()
			tasks[index].state = "run"
			started[index] = time.Now()
			mu.Unlock()

			report := func(detail string) {
				mu.Lock()
				tasks[index].Detail = detail
				mu.Unlock()
			}
			detail, err := fn(ctx, tasks[index].Name, report)

			mu.Lock()
			tasks[index].Elapsed = time.Since(started[index])
			tasks[index].Detail, tasks[index].Err = detail, err
			tasks[index].state = "ok"
			if err != nil {
				tasks[index].state = "fail"
			}
			name, elapsed := tasks[index].Name, tasks[index].Elapsed
			mu.Unlock()

			if err != nil {
				s.Event("%s: %v", name, err)
				return
			}
			s.Event("%s: %s (%.1fs)", name, detail, elapsed.Seconds())
		}(i)
	}

	finished := make(chan struct{})
	go func() {
		wg.Wait()
		close(finished)
	}()

	snapshot := func(tick int) []string {
		mu.Lock()
		defer mu.Unlock()
		return s.panel(title, tasks, tick)
	}

	if !s.Animated() {
		<-finished
		s.Commit([]string{StripANSI(snapshot(-1)[0])})
		return tasks
	}

	ticker := time.NewTicker(frameInterval)
	defer ticker.Stop()

	for tick := 0; ; tick++ {
		select {
		case <-finished:
			s.Commit(snapshot(-1))
			return tasks
		case <-ticker.C:
			s.Render(snapshot(tick))
		}
	}
}

func (s *Screen) panel(title string, tasks []Task, tick int) []string {
	done, failed := 0, 0
	for _, task := range tasks {
		switch task.state {
		case "ok":
			done++
		case "fail":
			done++
			failed++
		}
	}

	header := fmt.Sprintf("  %s %s  %s%d/%d%s", title, s.Bar(done, len(tasks), 20), s.C.Dim, done, len(tasks), s.C.Reset)
	if failed > 0 {
		header += fmt.Sprintf("  %s%d failed%s", s.C.Red, failed, s.C.Reset)
	}
	lines := []string{header}

	visible := tasks
	if len(tasks) > maxPanelRows {
		visible = nil
		for _, task := range tasks {
			if task.state == "run" || task.state == "fail" {
				visible = append(visible, task)
			}
		}
		if len(visible) > maxPanelRows {
			visible = visible[:maxPanelRows]
		}
	}

	width := 0
	for _, task := range visible {
		width = max(width, utf8.RuneCountInString(task.Name))
	}

	for _, task := range visible {
		var mark, detail string
		switch task.state {
		case "wait":
			mark, detail = s.C.Dim+"·"+s.C.Reset, s.C.Dim+"queued"+s.C.Reset
		case "run":
			frame := Frames[0]
			if tick >= 0 {
				frame = Frames[tick%len(Frames)]
			}
			mark, detail = s.C.Cyan+frame+s.C.Reset, s.C.Dim+task.Detail+s.C.Reset
		case "ok":
			mark = s.C.Green + "✓" + s.C.Reset
			detail = fmt.Sprintf("%s%s   %.1fs%s", s.C.Dim, task.Detail, task.Elapsed.Seconds(), s.C.Reset)
		case "fail":
			mark = s.C.Red + "✗" + s.C.Reset
			message := "failed"
			if task.Err != nil {
				message = oneLine(task.Err.Error())
			}
			detail = s.C.Red + message + s.C.Reset
		}
		lines = append(lines, fmt.Sprintf("   %s %s  %s", mark, padRight(task.Name, width), detail))
	}

	if hidden := len(tasks) - len(visible); hidden > 0 {
		lines = append(lines, fmt.Sprintf("   %s… %d more%s", s.C.Dim, hidden, s.C.Reset))
	}
	return lines
}

func (s *Screen) Bar(done, total, width int) string {
	if total <= 0 || width <= 0 {
		return ""
	}
	done = min(max(done, 0), total)
	filled := done * width / total

	tint := s.C.Yellow
	if done >= total {
		tint = s.C.Green
	}
	return fmt.Sprintf("%s▕%s%s%s%s%s▏%s %3d%%",
		s.C.Dim, s.C.Reset, tint, strings.Repeat("█", filled)+strings.Repeat("░", width-filled),
		s.C.Reset, s.C.Dim, s.C.Reset, done*100/total)
}

func (s *Screen) Sparkline(values []float64) string {
	marks := []rune("▁▂▃▄▅▆▇█")
	if len(values) == 0 {
		return ""
	}

	low, high := values[0], values[0]
	for _, value := range values {
		low, high = math.Min(low, value), math.Max(high, value)
	}
	span := high - low
	if span == 0 {
		span = 1
	}

	var out strings.Builder
	out.WriteString(s.C.Cyan)
	for _, value := range values {
		out.WriteRune(marks[int((value-low)/span*float64(len(marks)-1))])
	}
	out.WriteString(s.C.Reset)
	return out.String()
}

func StripANSI(line string) string {
	var out strings.Builder
	for i := 0; i < len(line); i++ {
		if line[i] != 0x1b {
			out.WriteByte(line[i])
			continue
		}
		if i+1 < len(line) && line[i+1] == '[' {
			i += 2
			for i < len(line) && (line[i] < 0x40 || line[i] > 0x7e) {
				i++
			}
			continue
		}
		i++
	}
	return out.String()
}

func Fit(line string, width int) string {
	if width <= 0 {
		return line
	}
	limit := width - 1

	var out strings.Builder
	visible := 0
	for i := 0; i < len(line); {
		if line[i] == 0x1b && i+1 < len(line) && line[i+1] == '[' {
			end := i + 2
			for end < len(line) && (line[end] < 0x40 || line[end] > 0x7e) {
				end++
			}
			end = min(end+1, len(line))
			out.WriteString(line[i:end])
			i = end
			continue
		}
		r, size := utf8.DecodeRuneInString(line[i:])
		if visible == limit {
			out.WriteString(esc + "0m")
			return out.String()
		}
		out.WriteRune(r)
		visible++
		i += size
	}
	return out.String()
}

func padRight(value string, width int) string {
	if gap := width - utf8.RuneCountInString(value); gap > 0 {
		return value + strings.Repeat(" ", gap)
	}
	return value
}

func oneLine(value string) string {
	value = strings.Join(strings.Fields(value), " ")
	if utf8.RuneCountInString(value) > 72 {
		return string([]rune(value)[:72]) + "…"
	}
	return value
}
