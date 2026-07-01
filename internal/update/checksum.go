package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
)

func (s *Service) verifyChecksum(ctx context.Context, assetName, archivePath string) error {
	release, err := s.latestRelease(ctx)
	if err != nil {
		return err
	}
	checksumAsset := selectChecksumAsset(release.Assets)
	if checksumAsset == nil {
		log.Printf("[update] Release 未提供 checksums.txt，跳过校验 asset=%s", assetName)
		return nil
	}
	temp, err := os.CreateTemp("", "denova-checksums-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	if err := temp.Close(); err != nil {
		return err
	}
	if err := s.downloadAsset(ctx, githubAssetDownloadURL(*checksumAsset), temp.Name(), 0, nil); err != nil {
		return err
	}
	expected, err := checksumForAsset(temp.Name(), assetName)
	if err != nil {
		return err
	}
	actual, err := fileSHA256(archivePath)
	if err != nil {
		return err
	}
	if !strings.EqualFold(expected, actual) {
		return fmt.Errorf("更新包校验失败: expected=%s actual=%s", expected, actual)
	}
	return nil
}

func checksumForAsset(path, assetName string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == assetName {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("checksums.txt 中缺少 %s", assetName)
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
