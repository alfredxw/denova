package update

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func extractArchive(archivePath, targetDir string) error {
	if strings.HasSuffix(archivePath, ".zip") {
		return extractZip(archivePath, targetDir)
	}
	if strings.HasSuffix(archivePath, ".tar.gz") {
		return extractTarGz(archivePath, targetDir)
	}
	return fmt.Errorf("不支持的更新包格式: %s", filepath.Base(archivePath))
}

func extractZip(archivePath, targetDir string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("打开 zip 更新包失败: %w", err)
	}
	defer reader.Close()
	for _, f := range reader.File {
		target, err := safeJoin(targetDir, f.Name)
		if err != nil {
			return err
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		if err := writeExtractedFile(target, rc, f.FileInfo().Mode()); err != nil {
			rc.Close()
			return err
		}
		rc.Close()
	}
	return nil
}

func extractTarGz(archivePath, targetDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("打开 tar.gz 更新包失败: %w", err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("读取 gzip 更新包失败: %w", err)
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		target, err := safeJoin(targetDir, header.Name)
		if err != nil {
			return err
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := writeExtractedFile(target, reader, header.FileInfo().Mode()); err != nil {
				return err
			}
		}
	}
	return nil
}

func safeJoin(root, name string) (string, error) {
	target := filepath.Join(root, filepath.Clean(name))
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("更新包包含非法路径: %s", name)
	}
	return target, nil
}

func writeExtractedFile(target string, reader io.Reader, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode.Perm())
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, reader)
	return err
}
