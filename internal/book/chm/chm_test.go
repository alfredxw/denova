package chm

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func sampleCHM(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "testdata", "sample.chm"))
	if err != nil {
		t.Fatalf("read sample.chm: %v", err)
	}
	return data
}

func TestExtractTopicsFollowsTableOfContents(t *testing.T) {
	archive, err := Open(sampleCHM(t))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer archive.Close()
	topics, err := archive.Topics()
	if err != nil {
		t.Fatalf("Topics: %v", err)
	}
	if len(topics) != len(fixtureTopics) {
		t.Fatalf("topics = %d, want %d", len(topics), len(fixtureTopics))
	}
	for i, topic := range fixtureTopics {
		if topics[i].Path != topic.name {
			t.Fatalf("topics[%d].Path = %q, want %q", i, topics[i].Path, topic.name)
		}
		if !bytes.Contains(topics[i].HTML, gbkBytes(t, topic.title)) {
			t.Fatalf("topics[%d] does not contain GBK title %q", i, topic.title)
		}
	}
	sitemap, ok := archive.Sitemap()
	if !ok {
		t.Fatalf("Sitemap missing for fixture")
	}
	if !bytes.Contains(sitemap, gbkBytes(t, fixtureTopics[0].title)) {
		t.Fatalf("sitemap does not contain GBK title %q", fixtureTopics[0].title)
	}
}

func TestExtractTopicsRejectsNonCHMData(t *testing.T) {
	for _, data := range [][]byte{
		nil,
		[]byte("plain text, not a compiled help file"),
		[]byte("<html><body>just an html page</body></html>"),
	} {
		if _, err := Open(data); err == nil {
			t.Fatalf("Open(%q) unexpectedly succeeded", data)
		}
	}
}
