package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"denova/config"
)

const browserSessionLifetime = 30 * 24 * time.Hour
const pairingLinkLifetime = 5 * time.Minute

// Browser credentials are independent of Agent journals. Only token digests are
// persisted; the credential snapshot prevents old logins returning after a restart or password change.
type browserSession struct {
	Username     string    `json:"username"`
	PasswordHash string    `json:"password_hash"`
	ExpiresAt    time.Time `json:"expires_at"`
}

func (s browserSession) valid(access config.RemoteAccessConfig, now time.Time) bool {
	return access.AllowLANAccess && s.Username == access.Username && s.PasswordHash == access.PasswordHash && now.Before(s.ExpiresAt)
}

type browserSessions struct {
	mu            sync.Mutex
	path          string
	sessions      map[string]browserSession
	loadErr       error
	pairingDigest string
	pairing       browserSession
}

func newBrowserSessions(dataDir string) *browserSessions {
	store := &browserSessions{path: filepath.Join(dataDir, "remote-access-sessions.json"), sessions: make(map[string]browserSession)}
	if dataDir == "" {
		store.loadErr = errors.New("remote access data directory is required")
		return store
	}
	data, err := os.ReadFile(store.path)
	if errors.Is(err, os.ErrNotExist) {
		return store
	}
	if err == nil {
		err = json.Unmarshal(data, &store.sessions)
	}
	if store.sessions == nil && err == nil {
		err = errors.New("invalid browser session store")
	}
	store.loadErr = err
	return store
}

func browserTokenDigest(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

func (s *browserSessions) authorized(token string, access config.RemoteAccessConfig) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loadErr != nil || token == "" {
		return false
	}
	session, ok := s.sessions[browserTokenDigest(token)]
	return ok && session.valid(access, time.Now())
}

func (s *browserSessions) issue(access config.RemoteAccessConfig) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.issueLocked(access)
}

func (s *browserSessions) issueLocked(access config.RemoteAccessConfig) (string, error) {
	if s.loadErr != nil {
		return "", s.loadErr
	}
	token := rand.Text()
	now := time.Now()
	next := make(map[string]browserSession)
	for digest, session := range s.sessions {
		if session.valid(access, now) {
			next[digest] = session
		}
	}
	next[browserTokenDigest(token)] = browserSession{Username: access.Username, PasswordHash: access.PasswordHash, ExpiresAt: now.Add(browserSessionLifetime)}
	if err := s.save(next); err != nil {
		return "", err
	}
	return token, nil
}

func (s *browserSessions) revoke(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loadErr != nil {
		return s.loadErr
	}
	digest := browserTokenDigest(token)
	if _, ok := s.sessions[digest]; !ok {
		return nil
	}
	next := make(map[string]browserSession, len(s.sessions))
	for key, session := range s.sessions {
		if key != digest {
			next[key] = session
		}
	}
	return s.save(next)
}

// Each new local link replaces the previous unused link; no long-lived invitation registry is needed.
func (s *browserSessions) createPairing(access config.RemoteAccessConfig) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loadErr != nil {
		return "", s.loadErr
	}
	token := rand.Text()
	s.pairingDigest = browserTokenDigest(token)
	s.pairing = browserSession{Username: access.Username, PasswordHash: access.PasswordHash, ExpiresAt: time.Now().Add(pairingLinkLifetime)}
	return token, nil
}

var errPairingInvalid = errors.New("pairing link is invalid or expired")

func (s *browserSessions) exchangePairing(token string, access config.RemoteAccessConfig) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if token == "" || s.pairingDigest != browserTokenDigest(token) || !s.pairing.valid(access, time.Now()) {
		return "", errPairingInvalid
	}
	credential, err := s.issueLocked(access)
	if err != nil {
		return "", err
	}
	s.pairingDigest = ""
	return credential, nil
}

// Publish memory only after the private file has been synced and atomically replaced.
func (s *browserSessions) save(next map[string]browserSession) error {
	data, err := json.Marshal(next)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(s.path), ".remote-access-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if err := file.Chmod(0o600); err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(file.Name(), s.path); err != nil {
		return err
	}
	s.sessions = next
	return nil
}
