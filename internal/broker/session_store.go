package broker

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// SessionStore persists the non-secret Session metadata (user id, expiry)
// to a JSON file. The access token never lives here — it goes to
// TokenStore (OS keychain).
type SessionStore struct {
	path string
}

// NewSessionStore returns a store backed by the given file path. The
// containing directory is created lazily on first Save.
func NewSessionStore(path string) *SessionStore {
	return &SessionStore{path: path}
}

// Load returns the persisted session. When no file exists yet, it returns
// a zero Session and ErrNotLoggedIn so callers can branch cleanly.
func (s *SessionStore) Load() (Session, error) {
	b, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Session{}, ErrNotLoggedIn
		}
		return Session{}, err
	}
	var sess Session
	if err := json.Unmarshal(b, &sess); err != nil {
		return Session{}, err
	}
	if sess.Provider == "" || sess.UserID == "" {
		return Session{}, ErrNotLoggedIn
	}
	return sess, nil
}

// Save atomically writes the session to disk with mode 0600.
func (s *SessionStore) Save(sess Session) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(sess, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// Clear removes the persisted session. Missing file is not an error.
func (s *SessionStore) Clear() error {
	if err := os.Remove(s.path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
