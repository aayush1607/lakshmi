package broker

import (
	"errors"

	"github.com/zalando/go-keyring"
)

// keyringService is the service name used in the OS keychain. All Lakshmi
// secrets live under this service, with the provider as the account name.
const keyringService = "sh.lakshmi"

// TokenStore abstracts access-token persistence so tests can swap in a
// memory implementation without hitting the OS keychain.
type TokenStore interface {
	Get(provider string) (string, error)
	Set(provider, token string) error
	Delete(provider string) error
}

// ErrNoToken is returned by TokenStore.Get when no token is stored for
// the given provider.
var ErrNoToken = errors.New("no token stored")

// NewKeyringTokenStore returns a TokenStore backed by the OS keychain
// (via github.com/zalando/go-keyring).
//
// On macOS this uses Keychain, on Linux the Secret Service, on Windows
// the Credential Manager. When the platform has no backend, tests can
// use NewMemoryTokenStore or call keyring.MockInit() in test setup.
func NewKeyringTokenStore() TokenStore { return keyringTokenStore{} }

type keyringTokenStore struct{}

func (keyringTokenStore) Get(provider string) (string, error) {
	v, err := keyring.Get(keyringService, provider)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return "", ErrNoToken
		}
		return "", err
	}
	return v, nil
}

func (keyringTokenStore) Set(provider, token string) error {
	if token == "" {
		return errors.New("empty token")
	}
	return keyring.Set(keyringService, provider, token)
}

func (keyringTokenStore) Delete(provider string) error {
	err := keyring.Delete(keyringService, provider)
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return err
	}
	return nil
}

// NewMemoryTokenStore returns an in-process TokenStore used by tests.
func NewMemoryTokenStore() TokenStore { return &memoryTokenStore{m: map[string]string{}} }

type memoryTokenStore struct {
	m map[string]string
}

func (s *memoryTokenStore) Get(p string) (string, error) {
	v, ok := s.m[p]
	if !ok {
		return "", ErrNoToken
	}
	return v, nil
}

func (s *memoryTokenStore) Set(p, t string) error {
	if t == "" {
		return errors.New("empty token")
	}
	s.m[p] = t
	return nil
}

func (s *memoryTokenStore) Delete(p string) error { delete(s.m, p); return nil }
