package session

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

func validateSessionID(id string) error {
	if strings.TrimSpace(id) == "" {
		return fmt.Errorf("会话 ID 不能为空")
	}
	for _, r := range id {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return fmt.Errorf("会话 ID 包含非法字符: %s", id)
	}
	return nil
}

func newSessionID() string {
	var b [6]byte
	if _, err := rand.Read(b[:]); err == nil {
		return "s-" + time.Now().UTC().Format("20060102150405") + "-" + hex.EncodeToString(b[:])
	}
	return fmt.Sprintf("s-%d", time.Now().UTC().UnixNano())
}

func newInterruptionID() string {
	return strings.TrimPrefix(newSessionID(), "s-")
}

func newContextCompactionID() string {
	return "cc-" + strings.TrimPrefix(newSessionID(), "s-")
}

func newContextCompactionRemovalID() string {
	return "ccr-" + strings.TrimPrefix(newSessionID(), "s-")
}
