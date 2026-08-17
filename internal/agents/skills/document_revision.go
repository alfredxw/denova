package skills

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// ErrRevisionConflict prevents a mutation based on stale Skill state.
var ErrRevisionConflict = errors.New("skill was updated by another operation")

func validateSkillDirectoryRevision(ctx context.Context, dir Directory, name, expected string, required bool) error {
	expected = strings.TrimSpace(expected)
	if expected == "" && !required {
		return nil
	}
	if expected == "" {
		return ErrRevisionConflict
	}
	root, err := openScopedSkillRoot(dir, name)
	if err != nil {
		return err
	}
	defer root.Close()
	revision, err := skillDirectoryRevision(ctx, root)
	if err != nil {
		return err
	}
	if revision != expected {
		return ErrRevisionConflict
	}
	return nil
}

func skillContentRevision(data []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

// skillDirectoryRevision fingerprints every entry that root deletion would
// remove. Paths, modes, sizes, mtimes, regular-file bytes, and symlink targets
// are framed independently so different directory layouts cannot collide.
func skillDirectoryRevision(ctx context.Context, root *os.Root) (string, error) {
	hasher := sha256.New()
	err := fs.WalkDir(root.FS(), ".", func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel := path.Clean(filepath.ToSlash(filePath))
		info, err := entry.Info()
		if err != nil {
			return err
		}
		writeSkillRevisionField(hasher, []byte(rel))
		writeSkillRevisionField(hasher, []byte(info.Mode().String()))
		writeSkillRevisionUint(hasher, uint64(info.Size()))
		writeSkillRevisionUint(hasher, uint64(info.ModTime().UnixNano()))
		switch {
		case info.Mode().IsRegular():
			file, err := root.Open(filepath.FromSlash(rel))
			if err != nil {
				return err
			}
			written, copyErr := io.Copy(hasher, file)
			closeErr := file.Close()
			if copyErr != nil || closeErr != nil {
				return errors.Join(copyErr, closeErr)
			}
			if written != info.Size() {
				return fmt.Errorf("skill file changed while computing revision: %s", rel)
			}
		case info.Mode()&os.ModeSymlink != 0:
			target, err := root.Readlink(filepath.FromSlash(rel))
			if err != nil {
				return err
			}
			writeSkillRevisionField(hasher, []byte(target))
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hasher.Sum(nil)), nil
}

func writeSkillRevisionField(writer io.Writer, value []byte) {
	writeSkillRevisionUint(writer, uint64(len(value)))
	_, _ = writer.Write(value)
}

func writeSkillRevisionUint(writer io.Writer, value uint64) {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], value)
	_, _ = writer.Write(encoded[:])
}
