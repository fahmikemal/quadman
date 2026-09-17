package ui

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestParseLogLine(t *testing.T) {
	raw := "2026-09-17T14:48:14+07:00 myhost myapp[12345]: failed to connect to database"
	entry := parseLogLine(raw, "myapp.service", 1)

	if entry.Timestamp != "2026-09-17T14:48:14+07:00" {
		t.Errorf("timestamp = %q", entry.Timestamp)
	}
	if entry.Host != "myhost" {
		t.Errorf("host = %q", entry.Host)
	}
	if entry.Process != "myapp[12345]" {
		t.Errorf("process = %q", entry.Process)
	}
	if entry.Message != "failed to connect to database" {
		t.Errorf("message = %q", entry.Message)
	}
	if entry.Level != "err" {
		t.Errorf("level = %q, want err", entry.Level)
	}
	if entry.LineNumber != 1 {
		t.Errorf("line = %d, want 1", entry.LineNumber)
	}

	// Plain line without iso header
	rawPlain := "random console output without timestamp"
	entryPlain := parseLogLine(rawPlain, "myapp.service", 2)
	if entryPlain.Message != rawPlain {
		t.Errorf("plain message = %q", entryPlain.Message)
	}
	if entryPlain.Level != "info" {
		t.Errorf("plain level = %q, want info", entryPlain.Level)
	}
}

func TestExportLogsText(t *testing.T) {
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "test.log")
	lines := []string{
		"line 1: service started",
		"line 2: connected to cache",
		"line 3: listening on :8080",
	}

	err := exportLogsText(outPath, "demo-web.service", lines)
	if err != nil {
		t.Fatalf("exportLogsText failed: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read file failed: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "Quadman Log Export: demo-web.service") {
		t.Errorf("header missing in export: %q", content)
	}
	for _, l := range lines {
		if !strings.Contains(content, l) {
			t.Errorf("missing log line %q in export", l)
		}
	}
}

func TestExportLogsJSONL(t *testing.T) {
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "test.jsonl")
	lines := []string{
		"2026-09-17T10:00:00Z host app[1]: initialization ok",
		"2026-09-17T10:00:01Z host app[1]: critical database timeout",
	}

	err := exportLogsJSONL(outPath, "demo-web.service", lines)
	if err != nil {
		t.Fatalf("exportLogsJSONL failed: %v", err)
	}

	f, err := os.Open(outPath)
	if err != nil {
		t.Fatalf("open file failed: %v", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var entries []LogEntry
	for scanner.Scan() {
		var e LogEntry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			t.Fatalf("json unmarshal failed: %v", err)
		}
		entries = append(entries, e)
	}

	if len(entries) != 2 {
		t.Fatalf("entries count = %d, want 2", len(entries))
	}
	if entries[0].Level != "info" {
		t.Errorf("entry 0 level = %q, want info", entries[0].Level)
	}
	if entries[1].Level != "crit" && entries[1].Level != "err" {
		t.Errorf("entry 1 level = %q, want crit/err", entries[1].Level)
	}
}

func TestDefaultExportFilename(t *testing.T) {
	name := defaultExportFilename("webapp@prod.service", "log")
	if !strings.HasPrefix(name, "webapp_at_prod-") || !strings.HasSuffix(name, ".log") {
		t.Errorf("unexpected filename: %s", name)
	}
}

func TestLogFilterGrep(t *testing.T) {
	m := withUnits(New(), "web-app")
	m.mode = modeDetail
	m.tab = tabJournal
	m.logLines = []string{
		"2026-09-17T10:00:00Z host app: starting server",
		"2026-09-17T10:00:01Z host app: error opening file",
		"2026-09-17T10:00:02Z host app: worker thread ready",
	}

	// Initially all 3 lines are visible
	if len(m.visibleLogLines()) != 3 {
		t.Fatalf("visible lines = %d, want 3", len(m.visibleLogLines()))
	}

	// Press 'F' to enter log filter mode
	model, _ := m.Update(tea.KeyPressMsg{Code: 'F', Text: "F"})
	mm := model.(Model)
	if !mm.filteringLogs {
		t.Fatal("pressing F should set filteringLogs to true")
	}

	// Type "error"
	for _, ch := range "error" {
		model, _ = mm.Update(tea.KeyPressMsg{Code: rune(ch), Text: string(ch)})
		mm = model.(Model)
	}

	// Press Enter to confirm filter
	model, _ = mm.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Text: "enter"})
	mm = model.(Model)

	if mm.filteringLogs {
		t.Error("enter should exit filteringLogs prompt")
	}
	if mm.logFilter != "error" {
		t.Errorf("logFilter = %q, want 'error'", mm.logFilter)
	}

	filtered := mm.visibleLogLines()
	if len(filtered) != 1 || !strings.Contains(filtered[0], "error opening file") {
		t.Errorf("visibleLogLines = %v, want 1 error line", filtered)
	}

	// Press Esc to clear filter
	model, _ = mm.Update(tea.KeyPressMsg{Code: tea.KeyEscape, Text: "esc"})
	mm = model.(Model)
	if mm.logFilter != "" {
		t.Errorf("esc should clear logFilter, got %q", mm.logFilter)
	}
	if len(mm.visibleLogLines()) != 3 {
		t.Errorf("after clearing filter, visibleLogLines = %d, want 3", len(mm.visibleLogLines()))
	}
}

func TestExportLogsRefusesReadonly(t *testing.T) {
	m := withUnits(New(), "web-app")
	m.readonly = true
	m.table.SetCursor(0)
	m.logLines = []string{"test log line"}

	model, _ := m.exportLogs("txt")
	mm := model.(Model)
	if !strings.Contains(mm.statusLine, "refused") || !strings.Contains(mm.statusLine, "readonly") {
		t.Errorf("export in readonly should report refusal: %q", mm.statusLine)
	}
}

func TestExportLogsClipboard(t *testing.T) {
	m := withUnits(New(), "web-app")
	m.table.SetCursor(0)
	m.logLines = []string{"line a", "line b"}

	model, cmd := m.exportLogs("clipboard")
	mm := model.(Model)
	if !strings.Contains(mm.statusLine, "copied 2 log lines") {
		t.Errorf("status line = %q", mm.statusLine)
	}
	if cmd == nil {
		t.Error("clipboard export should produce tea.Cmd for OSC52")
	}
}

func TestCycleLogPriority(t *testing.T) {
	m := withUnits(New(), "web-app")
	m.table.SetCursor(0)

	priorities := []string{"err", "warning", "info", ""}
	for _, expected := range priorities {
		model, _ := m.cycleLogPriority()
		m = model.(Model)
		if m.logPriority != expected {
			t.Errorf("logPriority = %q, want %q", m.logPriority, expected)
		}
	}
}
