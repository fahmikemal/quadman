package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// LogEntry is the structured representation of a single log line.
type LogEntry struct {
	Timestamp  string `json:"timestamp,omitempty"`
	Host       string `json:"host,omitempty"`
	Process    string `json:"process,omitempty"`
	Unit       string `json:"unit"`
	Message    string `json:"message"`
	Level      string `json:"level,omitempty"`
	LineNumber int    `json:"line"`
}

// shortIsoPattern matches standard short-iso systemd journal output:
// 2026-09-17T14:48:14+07:00 hostname process[123]: message
var shortIsoPattern = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}[^\s]+)\s+([^\s]+)\s+([^:]+):\s*(.*)$`)

// parseLogLine parses a raw journal line into a structured LogEntry.
func parseLogLine(raw string, unit string, lineNo int) LogEntry {
	entry := LogEntry{
		Unit:       unit,
		LineNumber: lineNo,
		Message:    raw,
	}

	matches := shortIsoPattern.FindStringSubmatch(raw)
	if len(matches) == 5 {
		entry.Timestamp = matches[1]
		entry.Host = matches[2]
		entry.Process = matches[3]
		entry.Message = matches[4]
	}

	// Detect log level/severity from content
	entry.Level = detectLogLevel(entry.Message)
	return entry
}

func detectLogLevel(msg string) string {
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "emerg"), strings.Contains(lower, "fatal"), strings.Contains(lower, "panic"):
		return "emerg"
	case strings.Contains(lower, "crit"):
		return "crit"
	case strings.Contains(lower, "error"), strings.Contains(lower, "err"), strings.Contains(lower, "failed"), strings.Contains(lower, "exception"):
		return "err"
	case strings.Contains(lower, "warn"), strings.Contains(lower, "warning"):
		return "warning"
	case strings.Contains(lower, "notice"):
		return "notice"
	case strings.Contains(lower, "debug"), strings.Contains(lower, "trace"):
		return "debug"
	default:
		return "info"
	}
}

// defaultExportFilename returns a clean filename for exporting logs.
func defaultExportFilename(unit string, ext string) string {
	safeUnit := strings.ReplaceAll(unit, "@", "_at_")
	safeUnit = strings.ReplaceAll(safeUnit, "/", "_")
	safeUnit = strings.TrimSuffix(safeUnit, ".service")
	timestamp := time.Now().Format("20060102-150405")
	return fmt.Sprintf("%s-%s.%s", safeUnit, timestamp, ext)
}

// uniqueExportPath appends -1, -2, ... when path already exists, so two
// exports in the same second never silently clobber each other.
func uniqueExportPath(path string) string {
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return path
	}
	ext := filepath.Ext(path)
	base := strings.TrimSuffix(path, ext)
	for i := 1; ; i++ {
		cand := fmt.Sprintf("%s-%d%s", base, i, ext)
		if _, err := os.Lstat(cand); os.IsNotExist(err) {
			return cand
		}
	}
}

// createExportFile exclusively creates the export destination, advancing
// the -N suffix while the name is taken. The O_EXCL open closes the
// check-then-create race between uniqueExportPath and the write: two
// concurrent exports can never clobber each other, and a pre-planted
// symlink at the destination is never followed. It returns the file and
// the final path (which may carry a higher suffix than base).
func createExportFile(base string) (*os.File, string, error) {
	for {
		cand := filepath.Clean(uniqueExportPath(base))
		// #nosec G304 -- path is the user-requested export destination, created exclusively
		f, err := os.OpenFile(cand, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			return f, cand, nil
		}
		if !os.IsExist(err) {
			return nil, "", err
		}
		// Lost a creation race: recompute, and the winner's file pushes
		// the suffix forward.
	}
}

// exportLogsText writes log lines as plain text to path. It returns the
// final path, which may carry a higher -N suffix than path when the
// destination was taken concurrently.
func exportLogsText(path string, unit string, lines []string) (string, error) {
	f, final, err := createExportFile(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	header := fmt.Sprintf("# Quadman Log Export: %s\n# Exported: %s\n# Total Lines: %d\n\n",
		unit, time.Now().Format(time.RFC3339), len(lines))
	if _, err := f.WriteString(header); err != nil {
		return "", err
	}
	for _, l := range lines {
		if _, err := fmt.Fprintf(f, "%s\n", l); err != nil {
			return "", err
		}
	}
	return final, nil
}

// exportLogsJSONL writes log lines as structured JSON Lines (ndjson) to path.
// It returns the final path, which may carry a higher -N suffix than path
// when the destination was taken concurrently.
func exportLogsJSONL(path string, unit string, lines []string) (string, error) {
	f, final, err := createExportFile(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	for i, l := range lines {
		entry := parseLogLine(l, unit, i+1)
		if err := enc.Encode(entry); err != nil {
			return "", err
		}
	}
	return final, nil
}
