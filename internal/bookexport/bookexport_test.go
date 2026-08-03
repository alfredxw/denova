package bookexport

import (
	"archive/zip"
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"denova/internal/book"
)

func sampleManuscript() book.Manuscript {
	return book.Manuscript{
		Title:  "星河边境",
		Author: "Denova",
		Volumes: []book.ManuscriptVolume{
			{
				Title: "第一卷 风起",
				Chapters: []book.ManuscriptChapter{
					{Title: "第一章 开局", Body: "天亮了。"},
					{Title: "第二章 追光", Body: "林川踏入雨夜。\n他抬头。"},
				},
			},
			{
				Title: "",
				Chapters: []book.ManuscriptChapter{
					{Title: "尾声", Body: "故事结束。"},
				},
			},
		},
	}
}

func TestRenderTextAssemblesReadableManuscript(t *testing.T) {
	content := RenderText(sampleManuscript())

	wantOrder := []string{
		"星河边境",
		"作者: Denova",
		"第一卷 风起",
		"第一章 开局",
		"天亮了。",
		"第二章 追光",
		"林川踏入雨夜。",
		"尾声",
		"故事结束。",
	}
	last := -1
	for _, item := range wantOrder {
		index := strings.Index(content, item)
		if index == -1 {
			t.Fatalf("text export missing %q:\n%s", item, content)
		}
		if index <= last {
			t.Fatalf("text export order mismatch around %q:\n%s", item, content)
		}
		last = index
	}
	if !strings.HasSuffix(content, "\n") {
		t.Fatalf("text export should end with a trailing newline")
	}
}

func TestRenderEPUBProducesValidZipWithChaptersAndCover(t *testing.T) {
	cover := testPNG(t)
	data, err := RenderEPUB(sampleManuscript(), cover)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 {
		t.Fatal("epub export is empty")
	}

	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("epub is not a valid zip: %v", err)
	}
	entries := map[string]bool{}
	for _, f := range zr.File {
		entries[f.Name] = true
	}
	// mimetype must be present and package/nav must exist for a valid EPUB.
	for _, required := range []string{"mimetype", "EPUB/package.opf", "EPUB/nav.xhtml", "EPUB/images/cover.png"} {
		if !entries[required] {
			t.Fatalf("epub missing required entry %q; entries=%v", required, entries)
		}
	}

	// The nav document should list volume and chapter titles for real chapters.
	nav := readZipEntry(t, zr, "EPUB/nav.xhtml")
	for _, want := range []string{"第一卷 风起", "第一章 开局", "第二章 追光", "尾声"} {
		if !strings.Contains(nav, want) {
			t.Fatalf("epub nav missing %q:\n%s", want, nav)
		}
	}
}

func TestRenderEPUBWithoutCoverSucceeds(t *testing.T) {
	data, err := RenderEPUB(sampleManuscript(), nil)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("epub is not a valid zip: %v", err)
	}
	for _, f := range zr.File {
		if f.Name == "EPUB/images/cover.png" {
			t.Fatal("epub should not embed a cover when none is provided")
		}
	}
}

func testPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{G: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode test png: %v", err)
	}
	return buf.Bytes()
}

func readZipEntry(t *testing.T, zr *zip.Reader, name string) string {
	t.Helper()
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %q: %v", name, err)
		}
		defer rc.Close()
		var buf bytes.Buffer
		if _, err := buf.ReadFrom(rc); err != nil {
			t.Fatalf("read %q: %v", name, err)
		}
		return buf.String()
	}
	t.Fatalf("zip entry %q not found", name)
	return ""
}
