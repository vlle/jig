package main

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	headLimit     = 200
	summaryLength = 100
	aboutLines    = 12
)

var (
	usageText     = regexp.MustCompile(`(?i)\busage:\s*(.+)`)
	argparseText  = regexp.MustCompile(`ArgumentParser\([^)]*description\s*=\s*(?:"([^"]+)"|'([^']+)')`)
	selfReference = regexp.MustCompile(`\$\{0##\*/\}|\$\(basename "?\$0"?\)|\$0|%s`)
	directiveLine = regexp.MustCompile(`(?i)^(shellcheck|-\*-|vim?:|@ts-|eslint|prettier|go:|\+build|nolint|noqa|type:|pylint:)`)
	legalLine     = regexp.MustCompile(`(?i)^(spdx|copyright|\(c\)|licen[sc]ed|license|author|code generated)`)
	decoration    = regexp.MustCompile(`^[\s#=*~_/+-]*$`)
	preludeLine   = regexp.MustCompile(`^(set\s+[-+]|shopt\s|IFS=|readonly\s|[A-Z_][A-Z0-9_]*=|['"]use strict['"])`)
)

func describeScript(path string) (summary, about string) {
	lines := headOf(path, headLimit)
	paragraphs := leadingComment(lines, filepath.Ext(path))
	if len(paragraphs) == 0 && filepath.Ext(path) == ".py" {
		paragraphs = docstring(lines)
	}
	if len(paragraphs) == 0 {
		return fallbackSummary(lines, filepath.Base(path)), ""
	}
	return summarize(paragraphs[0], path), aboutText(paragraphs)
}

func headOf(path string, limit int) []string {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = file.Close() }()

	var lines []string
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for scanner.Scan() && len(lines) < limit {
		lines = append(lines, scanner.Text())
	}
	return lines
}

func leadingComment(lines []string, ext string) [][]string {
	slashes := ext == ".go" || ext == ".js" || ext == ".mjs" || ext == ".cjs"

	var paragraphs [][]string
	var current []string
	flush := func() {
		if len(current) > 0 && !legalLine.MatchString(current[0]) {
			paragraphs = append(paragraphs, current)
		}
		current = nil
	}

	inBlock := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if i == 0 && strings.HasPrefix(trimmed, "#!") {
			continue
		}

		var text string
		switch {
		case inBlock:
			if before, _, closed := strings.Cut(trimmed, "*/"); closed {
				inBlock = false
				trimmed = before
			}
			text = strings.TrimSpace(strings.TrimLeft(trimmed, "*"))
		case slashes && strings.HasPrefix(trimmed, "/*"):
			body := strings.TrimLeft(strings.TrimPrefix(trimmed, "/*"), "*")
			if before, _, closed := strings.Cut(body, "*/"); closed {
				body = before
			} else {
				inBlock = true
			}
			text = strings.TrimSpace(body)
		case slashes && strings.HasPrefix(trimmed, "//"):
			text = strings.TrimSpace(strings.TrimLeft(trimmed, "/"))
		case !slashes && strings.HasPrefix(trimmed, "#"):
			text = strings.TrimSpace(strings.TrimLeft(trimmed, "#"))
		case trimmed == "":
			flush()
			continue
		case preludeLine.MatchString(trimmed):
			flush()
			continue
		default:
			flush()
			return paragraphs
		}

		switch {
		case decoration.MatchString(text):
			flush()
		case !directiveLine.MatchString(text):
			current = append(current, text)
		}
	}
	flush()
	return paragraphs
}

func docstring(lines []string) [][]string {
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		quote := ""
		for _, candidate := range []string{`"""`, `'''`} {
			if strings.HasPrefix(strings.TrimLeft(trimmed, "rRuU"), candidate) {
				quote = candidate
			}
		}
		if quote == "" {
			return nil
		}

		body := strings.TrimPrefix(strings.TrimLeft(trimmed, "rRuU"), quote)
		var collected []string
		for j := i; j < len(lines); j++ {
			if j > i {
				body = strings.TrimSpace(lines[j])
			}
			before, _, closed := strings.Cut(body, quote)
			collected = append(collected, strings.TrimSpace(before))
			if closed {
				break
			}
		}
		return splitParagraphs(collected)
	}
	return nil
}

func splitParagraphs(lines []string) [][]string {
	var paragraphs [][]string
	var current []string
	for _, line := range append(lines, "") {
		if line == "" {
			if len(current) > 0 && !legalLine.MatchString(current[0]) {
				paragraphs = append(paragraphs, current)
			}
			current = nil
			continue
		}
		current = append(current, line)
	}
	return paragraphs
}

func summarize(paragraph []string, path string) string {
	name := filepath.Base(path)
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	text := strings.TrimPrefix(strings.Join(paragraph, " "), "Command ")
	return sentence(dropLeadingName(text, name, stem, filepath.Base(filepath.Dir(path))))
}

func sentence(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	if end := strings.Index(text, ". "); end >= 20 {
		text = text[:end]
	}
	if strings.HasSuffix(text, ".") && !strings.HasSuffix(text, " .") && !strings.HasSuffix(text, "..") {
		text = strings.TrimSuffix(text, ".")
	}
	return clip(text, summaryLength)
}

func dropLeadingName(text string, names ...string) string {
	text = strings.Join(strings.Fields(text), " ")
	for _, candidate := range names {
		if len(text) <= len(candidate) || !strings.EqualFold(text[:len(candidate)], candidate) {
			continue
		}
		rest := strings.TrimLeft(text[len(candidate):], " ")
		for _, separator := range []string{"— ", "– ", "- ", ": "} {
			if after, found := strings.CutPrefix(rest, separator); found {
				return after
			}
		}
	}
	return text
}

func aboutText(paragraphs [][]string) string {
	var lines []string
	for i, paragraph := range paragraphs {
		if i > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, paragraph...)
		if len(lines) >= aboutLines {
			lines = lines[:aboutLines]
			break
		}
	}
	return strings.Join(lines, "\n")
}

func fallbackSummary(lines []string, name string) string {
	for _, line := range lines {
		if match := argparseText.FindStringSubmatch(line); match != nil {
			return clip(firstNonEmpty(match[1], match[2]), summaryLength)
		}
	}
	for _, line := range lines {
		match := usageText.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		usage := match[1]
		if end := strings.IndexAny(usage, "\"'`"); end >= 0 {
			usage = usage[:end]
		}
		usage = strings.TrimSuffix(strings.TrimSpace(usage), `\n`)
		usage = strings.TrimSpace(selfReference.ReplaceAllString(usage, name))
		if usage != "" {
			return clip("usage: "+usage, summaryLength)
		}
	}
	return ""
}

func clip(text string, limit int) string {
	if utf8.RuneCountInString(text) <= limit {
		return text
	}
	runes := []rune(text)[:limit]
	cut := string(runes)
	if space := strings.LastIndex(cut, " "); space > limit/2 {
		cut = cut[:space]
	}
	return strings.TrimRight(cut, " ,;:") + "…"
}
