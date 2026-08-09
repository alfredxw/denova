package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	logFileName          = "denova.log"
	logFileMaxMB         = 16
	logFileMaxBackups    = 4
	logFileMaxAgeDays    = 14
	logDirectoryMaxMB    = logFileMaxMB * (logFileMaxBackups + 1)
	logDirectoryMaxFiles = logFileMaxBackups + 1
)

func setupLogging(dir string) (string, io.Writer, func()) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "warning: initialize log directory: %v\n", err)
		return "", os.Stderr, func() {}
	}

	path := filepath.Join(dir, logFileName)
	if err := ensureWritableLogFile(path); err != nil {
		fmt.Fprintf(os.Stderr, "warning: initialize log file: %v\n", err)
		return "", os.Stderr, func() {}
	}
	if err := pruneLogDirectory(dir, logDirectoryMaxMB<<20, logDirectoryMaxFiles, path); err != nil {
		fmt.Fprintf(os.Stderr, "warning: prune old log files: %v\n", err)
	}

	writer := &lumberjack.Logger{
		Filename:   path,
		MaxSize:    logFileMaxMB,
		MaxBackups: logFileMaxBackups,
		MaxAge:     logFileMaxAgeDays,
		LocalTime:  true,
		Compress:   true,
	}
	output := io.MultiWriter(os.Stderr, writer)
	return path, output, func() {
		if err := writer.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "warning: close log file: %v\n", err)
		}
	}
}

func ensureWritableLogFile(path string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close %s after checking access: %w", path, err)
	}
	return nil
}

type retainedLogFile struct {
	path    string
	name    string
	size    int64
	modTime time.Time
}

// pruneLogDirectory bounds logs left by any Denova version. Lumberjack owns
// ongoing rotation; this startup pass also keeps pre-rotation daily logs from
// consuming disk forever.
func pruneLogDirectory(dir string, maxBytes int64, maxFiles int, currentPath string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	currentPath = filepath.Clean(currentPath)
	files := make([]retainedLogFile, 0, len(entries))
	var totalBytes int64
	for _, entry := range entries {
		if entry.IsDir() || !isLogFile(entry.Name()) {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if filepath.Clean(path) == currentPath {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}
		files = append(files, retainedLogFile{
			path:    path,
			name:    entry.Name(),
			size:    info.Size(),
			modTime: info.ModTime(),
		})
		totalBytes += info.Size()
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].modTime.Equal(files[j].modTime) {
			return files[i].name < files[j].name
		}
		return files[i].modTime.Before(files[j].modTime)
	})

	var errs []error
	for len(files) > 0 && (len(files) > maxFiles || totalBytes > maxBytes) {
		oldest := files[0]
		files = files[1:]
		if removeErr := os.Remove(oldest.path); removeErr != nil {
			errs = append(errs, fmt.Errorf("remove %s: %w", oldest.path, removeErr))
			continue
		}
		totalBytes -= oldest.size
	}
	return errors.Join(errs...)
}

func isLogFile(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	return strings.HasSuffix(name, ".log") || strings.HasSuffix(name, ".log.gz")
}
