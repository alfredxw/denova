package chat

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

func TestLogToolResultNeverWritesResultBody(t *testing.T) {
	original := log.Writer()
	var output bytes.Buffer
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(original) })

	const secret = "authorization=Bearer test-secret-value"
	logToolResult("bash", "call-1", "error: "+secret)
	logged := output.String()
	if strings.Contains(logged, secret) || strings.Contains(logged, "preview=") {
		t.Fatalf("tool result body leaked to logs: %q", logged)
	}
	if !strings.Contains(logged, "suspected_failure=true") || !strings.Contains(logged, "bytes=") {
		t.Fatalf("bounded failure classification missing: %q", logged)
	}
}
