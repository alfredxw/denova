package generation

import "testing"

func TestInferImageFormatPrefersBytesOverClaimedContentType(t *testing.T) {
	format, mimeType, err := inferImageFormat([]byte("\x89PNG\r\n\x1a\n"), "image/jpeg")
	if err != nil {
		t.Fatal(err)
	}
	if format != "png" || mimeType != "image/png" {
		t.Fatalf("inferred format = %q MIME type = %q", format, mimeType)
	}
}
