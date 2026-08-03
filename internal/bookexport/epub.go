package bookexport

import (
	"encoding/base64"
	"fmt"
	"html"
	"strings"

	"github.com/go-shiori/go-epub"

	"denova/internal/book"
)

// epubLang is the EPUB metadata language tag. Denova manuscripts are primarily
// Chinese; this only tags the package document and does not affect content.
const epubLang = "zh"

// RenderEPUB renders a manuscript into a valid EPUB 3.0 byte stream with a
// nested table of contents: each named volume is a section whose chapters are
// nested subsections, while chapters in the unnamed volume become top-level
// sections. When coverPNG is non-empty it is embedded as the EPUB cover image.
func RenderEPUB(m book.Manuscript, coverPNG []byte) ([]byte, error) {
	title := strings.TrimSpace(m.Title)
	if title == "" {
		title = "未命名书籍"
	}
	e, err := epub.NewEpub(title)
	if err != nil {
		return nil, fmt.Errorf("创建 EPUB 失败: %w", err)
	}
	e.SetLang(epubLang)
	if author := strings.TrimSpace(m.Author); author != "" {
		e.SetAuthor(author)
	}
	if len(coverPNG) > 0 {
		if err := setEPUBCover(e, coverPNG); err != nil {
			return nil, err
		}
	}

	for _, volume := range m.Volumes {
		if err := addEPUBVolume(e, volume); err != nil {
			return nil, err
		}
	}

	var buf strings.Builder
	if _, err := e.WriteTo(&buf); err != nil {
		return nil, fmt.Errorf("写入 EPUB 失败: %w", err)
	}
	return []byte(buf.String()), nil
}

// addEPUBVolume adds one volume to the EPUB. A named volume becomes a parent
// section with its chapters as nested subsections; the unnamed volume's chapters
// are added directly as top-level sections so no empty heading page appears.
func addEPUBVolume(e *epub.Epub, volume book.ManuscriptVolume) error {
	volumeTitle := strings.TrimSpace(volume.Title)
	if volumeTitle == "" {
		for _, chapter := range volume.Chapters {
			if _, err := e.AddSection(chapterXHTML(chapter), chapter.Title, "", ""); err != nil {
				return fmt.Errorf("写入章节 %q 失败: %w", chapter.Title, err)
			}
		}
		return nil
	}

	parentFilename, err := e.AddSection(headingXHTML(volumeTitle), volumeTitle, "", "")
	if err != nil {
		return fmt.Errorf("写入分卷 %q 失败: %w", volumeTitle, err)
	}
	for _, chapter := range volume.Chapters {
		if _, err := e.AddSubSection(parentFilename, chapterXHTML(chapter), chapter.Title, "", ""); err != nil {
			return fmt.Errorf("写入章节 %q 失败: %w", chapter.Title, err)
		}
	}
	return nil
}

func setEPUBCover(e *epub.Epub, coverPNG []byte) error {
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(coverPNG)
	imagePath, err := e.AddImage(dataURL, "cover.png")
	if err != nil {
		return fmt.Errorf("添加封面图片失败: %w", err)
	}
	if err := e.SetCover(imagePath, ""); err != nil {
		return fmt.Errorf("设置封面失败: %w", err)
	}
	return nil
}

// headingXHTML renders a standalone heading page (used for volume landing pages).
func headingXHTML(title string) string {
	return "<h1>" + html.EscapeString(title) + "</h1>"
}

// chapterXHTML renders a chapter as an XHTML body: a heading followed by one
// paragraph per non-empty source line, matching typical one-line-per-paragraph
// prose. All text is HTML-escaped since chapter bodies are plain text.
func chapterXHTML(chapter book.ManuscriptChapter) string {
	var b strings.Builder
	b.WriteString("<h1>")
	b.WriteString(html.EscapeString(chapter.Title))
	b.WriteString("</h1>\n")
	for _, line := range strings.Split(chapter.Body, "\n") {
		text := strings.TrimSpace(line)
		if text == "" {
			continue
		}
		b.WriteString("<p>")
		b.WriteString(html.EscapeString(text))
		b.WriteString("</p>\n")
	}
	return b.String()
}
