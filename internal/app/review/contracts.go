// Package review owns review-feedback validation, trusted context projection,
// and atomic cross-ledger consumption for writing and project Agent chats.
package reviewapp

import "denova/internal/book"

// Runtime is an immutable project snapshot. DocumentsEnabled distinguishes a
// Book project from a General project even though both have a file service.
type Runtime struct {
	Workspace        string
	StateRoot        string
	SessionID        string
	DocumentsEnabled bool
	BookService      *book.Service
}
