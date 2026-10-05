package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"os"
	"strconv"
	"time"

	"github.com/vlle/jig/internal/anim"
)

type demo struct {
	name  string
	title string
	run   func(*anim.Screen, float64) error
}

var demos = []demo{
	{"spinner", "spinner — unknown duration", demoSpinner},
	{"bar", "progress bar — known total", demoBar},
	{"fan", "fan-out panel — parallel tasks in one block", demoFan},
	{"spark", "sparkline — a live metric without a chart", demoSpark},
}

func cmdDemo(args []string) error {
	only, args := flagValue(args, "--only")
	rawSpeed, args := flagValue(args, "--speed")
	mode, args := flagValue(args, "--anim")
	if len(args) > 0 {
		return fmt.Errorf("unexpected arguments %q, usage: jig demo [--only spinner|bar|fan|spark] [--speed N] [--anim auto|always|never]", args)
	}

	speed := 1.0
	if rawSpeed != "" {
		parsed, err := strconv.ParseFloat(rawSpeed, 64)
		if err != nil || parsed <= 0 {
			return fmt.Errorf("--speed must be a positive number, got %q", rawSpeed)
		}
		speed = parsed
	}
	if mode == "" {
		mode = os.Getenv("JIG_ANIM")
	}

	selected := demos
	if only != "" {
		selected = nil
		for _, item := range demos {
			if item.name == only {
				selected = append(selected, item)
			}
		}
		if len(selected) == 0 {
			return fmt.Errorf("unknown --only=%q, expected spinner, bar, fan or spark", only)
		}
	}

	s, err := anim.NewScreen(anim.Options{Mode: mode, Tool: "demo"})
	if err != nil {
		return err
	}
	defer s.Close()

	if !s.Animated() {
		s.Note("animation is off (not a TTY, NO_COLOR or TERM=dumb): one plain line per finished step")
	}

	for i, item := range selected {
		s.Note("\n  %s%d · %s%s", s.C.Dim, i+1, item.title, s.C.Reset)
		_ = item.run(s, speed)
	}
	s.Note("")
	return nil
}

func pause(speed float64, base time.Duration) {
	time.Sleep(time.Duration(float64(base) / speed))
}

func demoSpinner(s *anim.Screen, speed float64) error {
	return s.Step("connecting to the database", func(report func(string)) error {
		for _, phase := range []string{"resolving host", "tls handshake", "authenticating", "warming pool"} {
			report(phase)
			pause(speed, 700*time.Millisecond)
		}
		report("pool ready")
		return nil
	})
}

func demoBar(s *anim.Screen, speed float64) error {
	const total = 320
	return s.Count("reading rows", total, func(advance func(int)) error {
		for done := 0; done < total; done += 4 {
			advance(4)
			pause(speed, 25*time.Millisecond)
		}
		return nil
	})
}

func demoFan(s *anim.Screen, speed float64) error {
	names := make([]string, 8)
	for i := range names {
		names[i] = fmt.Sprintf("shard-%d", i+1)
	}
	random := rand.New(rand.NewSource(7))
	delays := make(map[string]time.Duration, len(names))
	for _, name := range names {
		delays[name] = time.Duration(600+random.Intn(2200)) * time.Millisecond
	}

	tasks := s.Fan(context.Background(), "shards", names, 4, func(ctx context.Context, name string, report func(string)) (string, error) {
		report("querying")
		pause(speed, delays[name]/2)
		report("reading rows")
		pause(speed, delays[name]/2)
		if name == "shard-6" {
			return "", errors.New("connection refused")
		}
		return fmt.Sprintf("%d rows", 40+int(delays[name].Milliseconds()/9)), nil
	})

	for _, task := range tasks {
		if task.Err != nil {
			return task.Err
		}
	}
	return nil
}

func demoSpark(s *anim.Screen, speed float64) error {
	const width = 36
	history := make([]float64, 0, width)
	peak := 0.0

	for i := range 90 {
		value := 300 + 220*math.Sin(float64(i)/6) + float64(i%5)*18
		peak = math.Max(peak, value)
		history = append(history, value)
		if len(history) > width {
			history = history[1:]
		}
		s.Render([]string{fmt.Sprintf("  %srps%s %s   %s%4.0f/s%s", s.C.Dim, s.C.Reset, s.Sparkline(history), s.C.Bold, value, s.C.Reset)})
		pause(speed, 40*time.Millisecond)
	}

	s.Commit([]string{fmt.Sprintf("  %srps%s %s   %speak %.0f/s%s", s.C.Dim, s.C.Reset, s.Sparkline(history), s.C.Dim, peak, s.C.Reset)})
	return nil
}
