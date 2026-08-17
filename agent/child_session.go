package agent

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	agentsession "github.com/alfredxw/denova/agent/session"
)

// ParentSessionAttribute is the reserved child Session identity edge used by
// Agent lifecycle tree operations. Values are produced by ChildSessionAttributes;
// applications must not place mutable display or per-cycle data in this field.
const ParentSessionAttribute = "parent_session"

// ChildSessionAttributes encodes the exact canonical parent Session identity.
// A child may add other immutable selector attributes before it is opened.
func ChildSessionAttributes(parent SessionKey) (map[string]string, error) {
	parent, err := agentsession.NormalizeKey(parent)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(parent)
	if err != nil {
		return nil, fmt.Errorf("encode parent Agent Session: %w", err)
	}
	return map[string]string{
		ParentSessionAttribute: base64.RawURLEncoding.EncodeToString(encoded),
	}, nil
}

// ParentSessionKey decodes the exact parent identity of a child Session.
func ParentSessionKey(child SessionKey) (SessionKey, error) {
	encoded := strings.TrimSpace(child.Attributes[ParentSessionAttribute])
	if encoded == "" {
		return SessionKey{}, errors.New("Agent child Session has no parent identity")
	}
	data, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return SessionKey{}, fmt.Errorf("decode parent Agent Session: %w", err)
	}
	var parent SessionKey
	if err := json.Unmarshal(data, &parent); err != nil {
		return SessionKey{}, fmt.Errorf("decode parent Agent Session: %w", err)
	}
	parent, err = agentsession.NormalizeKey(parent)
	if err != nil {
		return SessionKey{}, fmt.Errorf("decode parent Agent Session: %w", err)
	}
	return parent, nil
}
