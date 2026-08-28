package handlers

import (
	"os/user"
	"testing"
)

func TestResolveBookAuthor(t *testing.T) {
	tests := []struct {
		name    string
		author  string
		current *user.User
		want    string
	}{
		{name: "explicit author", author: "  Pen Name  ", current: &user.User{Name: "Creator"}, want: "Pen Name"},
		{name: "current user display name", current: &user.User{Name: "  Creator Name  ", Username: "creator"}, want: "Creator Name"},
		{name: "current username", current: &user.User{Username: "  creator  "}, want: "creator"},
		{name: "generic fallback", want: "User"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := resolveBookAuthor(test.author, test.current); got != test.want {
				t.Fatalf("resolveBookAuthor() = %q, want %q", got, test.want)
			}
		})
	}
}
