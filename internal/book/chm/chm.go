// Package chm extracts topic files from Microsoft Compiled HTML Help (CHM,
// ITSF/LZX) containers in reading order. It only opens the container and
// surfaces its structure; topic text decoding and HTML conversion stay with
// the caller.
package chm

import (
	"fmt"
	"path"
	"strings"

	"github.com/gen2brain/folio/html"
)

// MaxExtractedBytes bounds the total uncompressed topic payload an Archive
// returns, so a crafted container cannot expand without bound. Real reference
// manuals decompress well above their file size, so this is generous by design.
const MaxExtractedBytes = 256 << 20

// Topic is one topic file of a CHM container, in spine order: the .hhc table
// of contents order first, then any remaining topics in container directory
// order.
type Topic struct {
	// Path is the topic path inside the container, e.g. "01.html".
	Path string
	// HTML holds the raw topic payload, still in its source encoding.
	HTML []byte
}

// Archive is an opened CHM container.
type Archive struct {
	doc *html.Document
}

// Open parses data as a CHM container. The container kind is verified so a
// renamed file of another format fails with a clear error.
func Open(data []byte) (*Archive, error) {
	doc, err := html.Load(data)
	if err != nil {
		return nil, fmt.Errorf("无法解析 CHM 容器: %w", err)
	}
	if doc.Kind() != html.KindCHM {
		doc.Close()
		return nil, fmt.Errorf("文件不是 CHM 容器")
	}
	return &Archive{doc: doc}, nil
}

// Close releases the container.
func (a *Archive) Close() error {
	return a.doc.Close()
}

// Topics returns the text topics of the container in spine order. Empty
// topics are skipped and the total uncompressed payload is capped at
// MaxExtractedBytes.
func (a *Archive) Topics() ([]Topic, error) {
	spine := a.doc.Spine()
	topics := make([]Topic, 0, len(spine))
	total := 0
	for _, item := range spine {
		payload, err := a.doc.Read(item.Path)
		if err != nil {
			return nil, fmt.Errorf("读取 CHM 主题 %s 失败: %w", item.Path, err)
		}
		if len(payload) == 0 {
			continue
		}
		total += len(payload)
		if total > MaxExtractedBytes {
			return nil, fmt.Errorf("CHM 解压后内容超过 %d 字节上限", MaxExtractedBytes)
		}
		topics = append(topics, Topic{Path: item.Path, HTML: payload})
	}
	return topics, nil
}

// Sitemap returns the raw bytes of the container's first .hhc table of
// contents, still in its source encoding, and whether one exists. Callers
// decode it themselves because CHM files rarely store the sitemap in UTF-8.
func (a *Archive) Sitemap() ([]byte, bool) {
	for _, item := range a.doc.Manifest() {
		if strings.EqualFold(path.Ext(item.Path), ".hhc") {
			raw, err := a.doc.Read(item.Path)
			if err != nil || len(raw) == 0 {
				return nil, false
			}
			return raw, true
		}
	}
	return nil, false
}
