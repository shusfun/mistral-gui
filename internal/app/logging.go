package app

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

type rotatingLog struct {
	mu   sync.Mutex
	path string
	file *os.File
	size int64
}

func (w *rotatingLog) Write(b []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil || w.size+int64(len(b)) > 8<<20 {
		if w.file != nil {
			_ = w.file.Close()
			_ = os.Remove(w.path + ".previous")
			_ = os.Rename(w.path, w.path+".previous")
			w.file = nil
		}
		f, e := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if e != nil {
			return 0, e
		}
		w.file = f
		if info, e := f.Stat(); e == nil {
			w.size = info.Size()
		}
	}
	n, e := w.file.Write(b)
	w.size += int64(n)
	return n, e
}
func (w *rotatingLog) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file != nil {
		return w.file.Close()
	}
	return nil
}
func (a *App) EnableLogging() io.Closer {
	writer := &rotatingLog{path: filepath.Join(a.dataDir, "manager.log")}
	slog.SetDefault(slog.New(slog.NewJSONHandler(io.MultiWriter(writer, os.Stderr), nil)))
	return writer
}
