package conversationjournal

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"denova/internal/localfs"
)

func writeAll(file *os.File, data []byte) error {
	for len(data) > 0 {
		written, err := file.Write(data)
		if err != nil {
			return err
		}
		if written == 0 {
			return io.ErrShortWrite
		}
		data = data[written:]
	}
	return nil
}

func appendAndSync(path string, validOffset int64, needsNewline bool, line []byte) (Location, int64, error) {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_APPEND, 0)
	if err != nil {
		return Location{}, 0, fmt.Errorf("open conversation journal for append: %w", err)
	}
	start := validOffset
	payload := make([]byte, 0, len(line)+2)
	if needsNewline {
		payload = append(payload, '\n')
		start++
	}
	payload = append(payload, line...)
	payload = append(payload, '\n')
	writeErr := writeAll(file, payload)
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		return Location{}, 0, errors.Join(writeErr, syncErr, closeErr)
	}
	if err := localfs.SyncDirectory(filepath.Dir(path)); err != nil {
		return Location{}, 0, err
	}
	return Location{Offset: start, Length: len(line)}, validOffset + int64(len(payload)), nil
}

func preserveAndTruncateTail(path string, validOffset int64, tail []byte) error {
	if len(tail) == 0 {
		return nil
	}
	base := fmt.Sprintf("%s.incomplete-%d-%d", path, time.Now().UTC().UnixNano(), os.Getpid())
	backup, err := os.OpenFile(base, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create conversation journal tail backup: %w", err)
	}
	writeErr := writeAll(backup, tail)
	syncErr := backup.Sync()
	closeErr := backup.Close()
	if writeErr != nil || syncErr != nil || closeErr != nil {
		return errors.Join(writeErr, syncErr, closeErr)
	}
	file, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	truncateErr := file.Truncate(validOffset)
	if truncateErr == nil {
		truncateErr = file.Sync()
	}
	closeErr = file.Close()
	if truncateErr != nil || closeErr != nil {
		return errors.Join(truncateErr, closeErr)
	}
	if err := localfs.SyncDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	return nil
}

func validateIdentity(identity Identity) error {
	identity.ID = strings.TrimSpace(identity.ID)
	identity.Generation = strings.TrimSpace(identity.Generation)
	if identity.ID == "" || identity.Generation == "" {
		return fmt.Errorf("conversation journal identity and generation are required")
	}
	return nil
}
