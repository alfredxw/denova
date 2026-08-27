package agent

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Attachment is a durable, provider-neutral copy of a file supplied with one
// user message. Path points to application-owned user data, never the caller's
// original file.
type Attachment struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	MediaType string `json:"media_type,omitempty"`
	Size      int64  `json:"size"`
	Path      string `json:"path,omitempty"`
	// SHA256 binds provider input to the immutable bytes accepted from the user.
	SHA256 string `json:"sha256,omitempty"`
}

// IsNativeImageMediaType reports whether every built-in multimodal protocol
// can safely represent the image as an inline user-input part.
func IsNativeImageMediaType(mediaType string) bool {
	switch strings.ToLower(strings.TrimSpace(mediaType)) {
	case "image/jpeg", "image/png", "image/gif", "image/webp":
		return true
	default:
		return false
	}
}

// UserMessageWithAttachments constructs one canonical user input without
// changing the user-authored text.
func UserMessageWithAttachments(content string, attachments []Attachment) *Message {
	return &Message{Role: User, Content: content, Attachments: cloneAttachments(attachments)}
}

// ModelUserContent appends the stable filesystem contract for attached copies.
// Native image adapters also send supported images as image content parts.
func ModelUserContent(message *Message) string {
	if message == nil || len(message.Attachments) == 0 {
		if message == nil {
			return ""
		}
		return message.Content
	}
	var builder strings.Builder
	if content := strings.TrimSpace(message.Content); content != "" {
		builder.WriteString(content)
		builder.WriteString("\n\n")
	}
	builder.WriteString("# Attached files\n\n")
	builder.WriteString("These are immutable input copies owned by the application. Read them with available filesystem or shell tools when useful. Never modify or delete an attached input. To edit its content, copy it into the workspace or create a new output artifact.\n")
	for _, attachment := range message.Attachments {
		builder.WriteString("\n- name: ")
		builder.WriteString(strconv.Quote(attachment.Name))
		builder.WriteString("\n  path: ")
		builder.WriteString(strconv.Quote(attachment.Path))
		if attachment.MediaType != "" {
			builder.WriteString("\n  media_type: ")
			builder.WriteString(strconv.Quote(attachment.MediaType))
		}
		builder.WriteString("\n  size_bytes: ")
		builder.WriteString(strconv.FormatInt(attachment.Size, 10))
	}
	return builder.String()
}

// AttachmentDataURL loads an application-owned image copy at provider request
// time, keeping binary payloads out of the durable transcript.
func AttachmentDataURL(attachment Attachment) (string, error) {
	if !IsNativeImageMediaType(attachment.MediaType) {
		return "", fmt.Errorf("attachment %q is not a supported native image", attachment.Name)
	}
	encoded, err := AttachmentBase64(attachment)
	if err != nil {
		return "", err
	}
	return "data:" + attachment.MediaType + ";base64," + encoded, nil
}

// AttachmentBase64 loads one native image for protocols whose source schema
// carries media type and base64 data separately.
func AttachmentBase64(attachment Attachment) (string, error) {
	if !IsNativeImageMediaType(attachment.MediaType) {
		return "", fmt.Errorf("attachment %q is not a supported native image", attachment.Name)
	}
	data, err := os.ReadFile(attachment.Path)
	if err != nil {
		return "", fmt.Errorf("read attached image %q: %w", attachment.Name, err)
	}
	digest := sha256.Sum256(data)
	if !strings.EqualFold(hex.EncodeToString(digest[:]), strings.TrimSpace(attachment.SHA256)) {
		return "", fmt.Errorf("attached image %q immutable copy changed", attachment.Name)
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

func cloneAttachments(values []Attachment) []Attachment {
	return append([]Attachment(nil), values...)
}

func attachmentsFromMessages(messages []*Message) []Attachment {
	var attachments []Attachment
	seen := make(map[string]struct{})
	for _, message := range messages {
		if message == nil {
			continue
		}
		for _, attachment := range message.Attachments {
			path := strings.TrimSpace(attachment.Path)
			if path == "" {
				continue
			}
			if _, exists := seen[path]; exists {
				continue
			}
			seen[path] = struct{}{}
			attachments = append(attachments, attachment)
		}
	}
	return attachments
}
