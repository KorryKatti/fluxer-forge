// Package log sets up slog: readable lines on stdout and JSONL in a daily
// file under logs/, keeping the last seven days.
package log

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Setup starts logging. The returned function closes the log file.
// If the directory cannot be created, the caller still gets console logging.
func Setup(dir string) (func(), error) {
	writer := &dailyWriter{dir: dir, keep: 7}
	if err := writer.openToday(); err != nil {
		return func() {}, err
	}

	console := slog.NewTextHandler(os.Stdout, nil)
	file := slog.NewJSONHandler(writer, nil)
	slog.SetDefault(slog.New(multiHandler{console, file}))

	return func() { writer.Close() }, nil
}

// multiHandler sends every record to all handlers.
type multiHandler []slog.Handler

func (m multiHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, h := range m {
		if h.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (m multiHandler) Handle(ctx context.Context, record slog.Record) error {
	var firstErr error
	for _, h := range m {
		if err := h.Handle(ctx, record.Clone()); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (m multiHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := make(multiHandler, len(m))
	for i, h := range m {
		next[i] = h.WithAttrs(attrs)
	}
	return next
}

func (m multiHandler) WithGroup(name string) slog.Handler {
	next := make(multiHandler, len(m))
	for i, h := range m {
		next[i] = h.WithGroup(name)
	}
	return next
}

// dailyWriter writes to logs/bot-YYYY-MM-DD.log and switches files at
// midnight. Keeping the last seven is a small cleanup after each switch.
type dailyWriter struct {
	mu   sync.Mutex
	dir  string
	keep int

	day  string
	file *os.File
}

func (w *dailyWriter) openToday() error {
	return w.open(time.Now())
}

func (w *dailyWriter) open(now time.Time) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	day := now.Format("2006-01-02")
	if w.file != nil && w.day == day {
		return nil
	}
	if w.file != nil {
		w.file.Close()
		w.file = nil
	}

	if err := os.MkdirAll(w.dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(w.dir, "bot-"+day+".log")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}

	w.file = file
	w.day = day
	w.prune()
	return nil
}

func (w *dailyWriter) Write(p []byte) (int, error) {
	if err := w.open(time.Now()); err != nil {
		return 0, err
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	return w.file.Write(p)
}

func (w *dailyWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

// prune deletes the oldest bot-*.log files, keeping w.keep of them.
func (w *dailyWriter) prune() {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return
	}

	var names []string
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, "bot-") && strings.HasSuffix(name, ".log") {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	for len(names) > w.keep {
		os.Remove(filepath.Join(w.dir, names[0]))
		names = names[1:]
	}
}
