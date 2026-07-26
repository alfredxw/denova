package skills

import (
	cryptorand "crypto/rand"
	"fmt"
	"os"
	"path"
	"path/filepath"
)

func atomicWriteSkillFile(root *os.Root, rel string, data []byte, mode os.FileMode) error {
	dir := path.Dir(filepath.ToSlash(rel))
	base := path.Base(filepath.ToSlash(rel))
	var random [8]byte
	if _, err := cryptorand.Read(random[:]); err != nil {
		return err
	}
	tempRel := path.Join(dir, fmt.Sprintf(".%s.denova-%x.tmp", base, random[:]))
	tempPath := filepath.FromSlash(tempRel)
	targetPath := filepath.FromSlash(rel)
	file, err := root.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	removeTemp := true
	defer func() {
		_ = file.Close()
		if removeTemp {
			_ = root.Remove(tempPath)
		}
	}()
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := root.Rename(tempPath, targetPath); err != nil {
		return err
	}
	removeTemp = false
	return nil
}

func atomicCreateSkillFile(root *os.Root, rel string, data []byte, mode os.FileMode) error {
	dir := path.Dir(filepath.ToSlash(rel))
	base := path.Base(filepath.ToSlash(rel))
	var random [8]byte
	if _, err := cryptorand.Read(random[:]); err != nil {
		return err
	}
	tempRel := path.Join(dir, fmt.Sprintf(".%s.denova-%x.tmp", base, random[:]))
	tempPath := filepath.FromSlash(tempRel)
	targetPath := filepath.FromSlash(rel)
	file, err := root.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	removeTemp := true
	defer func() {
		_ = file.Close()
		if removeTemp {
			_ = root.Remove(tempPath)
		}
	}()
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := root.Link(tempPath, targetPath); err != nil {
		return err
	}
	// The target is now a durable hard link to the fully synced inode. Temp
	// cleanup is best-effort and must not turn a successful create into an
	// ambiguous error that invites a duplicate retry.
	return nil
}
