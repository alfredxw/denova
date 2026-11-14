package interactiveapp

import (
	"denova/internal/book/lore"
)

// residentLoreReader is the narrow storage boundary required to assemble one
// revision-consistent stable context shared by interactive helper agents.
type residentLoreReader interface {
	List() ([]lore.Item, error)
	ResidentContextMarkdown() (string, error)
	Revision() (string, error)
}

type residentLoreSnapshot struct {
	Content   string
	BodyBytes int
	IDs       []string
	Revision  string
}