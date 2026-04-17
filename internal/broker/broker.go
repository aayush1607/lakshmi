// Package broker defines the brokerage-agnostic contract used by Lakshmi.
//
// Sprint 1 only implements a Zerodha client (see internal/broker/zerodha),
// but this package owns the types so that swapping brokers later — or
// adding paper-trading shims — is a one-file change at wiring time.
//
// The Broker interface is deliberately read-only: Sprint 1 never places
// or modifies trades. That invariant is enforced by the interface shape.
package broker

import (
	"context"
	"errors"
	"time"
)

// Provider names a supported brokerage.
type Provider string

// ProviderZerodha is the only implemented provider in Sprint 1.
const ProviderZerodha Provider = "zerodha"

// Session describes an authenticated link to a brokerage account. It holds
// only non-secret metadata; the access token lives in TokenStore.
type Session struct {
	Provider  Provider  `json:"provider"`
	UserID    string    `json:"user_id"`
	UserName  string    `json:"user_name"`
	Expiry    time.Time `json:"expiry"`     // UTC
	FetchedAt time.Time `json:"fetched_at"` // UTC, when the session was issued
}

// Active reports whether the session is still within its validity window.
func (s Session) Active(now time.Time) bool {
	if s.Provider == "" || s.UserID == "" {
		return false
	}
	return now.Before(s.Expiry)
}

// Holding is a single equity position held in the user's account. F1.2
// defines the type so the Broker interface can be shaped; F1.3 actually
// fetches and renders holdings.
type Holding struct {
	Symbol   string
	Exchange string
	ISIN     string
	Quantity int
	AvgCost  float64
	LTP      float64
	Close    float64
	Product  string
}

// Sentinel errors returned by Broker implementations.
var (
	ErrNotLoggedIn    = errors.New("not logged in")
	ErrSessionExpired = errors.New("session expired")
	ErrNotImplemented = errors.New("not implemented")
	ErrLoginCancelled = errors.New("login cancelled")
)

// Broker is the read-only interface Lakshmi uses to talk to a brokerage.
// Implementations MUST NOT expose any order-placing or cancellation
// methods in Sprint 1 — this is the read-only guarantee the spec makes.
type Broker interface {
	Provider() Provider
	Login(ctx context.Context) (Session, error)
	Session() (Session, error)
	Logout() error
	Holdings(ctx context.Context) ([]Holding, error)
}
