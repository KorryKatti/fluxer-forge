package log

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSetupWritesJSONToDailyFile(t *testing.T) {
	dir := t.TempDir()
	closeLogs, err := Setup(dir)
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}

	slog.Info("hello", "answer", 42)
	closeLogs()

	path := filepath.Join(dir, "bot-"+time.Now().Format("2006-01-02")+".log")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(data), `"msg":"hello"`) || !strings.Contains(string(data), `"answer":42`) {
		t.Errorf("log file does not contain the record: %s", data)
	}
}

func TestPruneKeepsSevenFiles(t *testing.T) {
	dir := t.TempDir()
	for _, day := range []string{
		"2026-10-01", "2026-10-02", "2026-10-03", "2026-10-04", "2026-10-05",
		"2026-10-06", "2026-10-07", "2026-10-08", "2026-10-09",
	} {
		os.WriteFile(filepath.Join(dir, "bot-"+day+".log"), []byte("old\n"), 0o644)
	}

	writer := &dailyWriter{dir: dir, keep: 7}
	if err := writer.open(time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("open: %v", err)
	}
	writer.Close()

	entries, _ := os.ReadDir(dir)
	if len(entries) != 7 {
		t.Errorf("kept %d files, want 7", len(entries))
	}
	if _, err := os.Stat(filepath.Join(dir, "bot-2026-10-01.log")); !os.IsNotExist(err) {
		t.Errorf("oldest file was not deleted")
	}
}
