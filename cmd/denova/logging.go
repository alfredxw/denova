package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

func setupLogging(dir string) (string, func()) {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds | log.Lshortfile)
	writer := newDailyLogWriter(dir)
	if err := writer.openFor(time.Now()); err != nil {
		fmt.Fprintf(os.Stderr, "警告: 初始化日志文件失败: %v\n", err)
		log.SetOutput(os.Stderr)
		return "", func() {}
	}
	log.SetOutput(io.MultiWriter(os.Stderr, writer))
	return writer.Path(), func() {
		if err := writer.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "警告: 关闭日志文件失败: %v\n", err)
		}
	}
}

type dailyLogWriter struct {
	dir  string
	mu   sync.Mutex
	date string
	file *os.File
	path string
}

func newDailyLogWriter(dir string) *dailyLogWriter {
	return &dailyLogWriter{dir: dir}
}

func (w *dailyLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.openFor(time.Now()); err != nil {
		return 0, err
	}
	return w.file.Write(p)
}

func (w *dailyLogWriter) Path() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.path
}

func (w *dailyLogWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

func (w *dailyLogWriter) openFor(now time.Time) error {
	date := now.Format("2006-01-02")
	if w.file != nil && w.date == date {
		return nil
	}
	if err := os.MkdirAll(w.dir, 0o755); err != nil {
		return fmt.Errorf("创建日志目录 %s 失败: %w", w.dir, err)
	}

	path := filepath.Join(w.dir, date+".log")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("打开日志文件 %s 失败: %w", path, err)
	}
	if w.file != nil {
		_ = w.file.Close()
	}
	w.file = file
	w.date = date
	w.path = path
	return nil
}
