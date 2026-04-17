// Package config loads Lakshmi configuration. Sprint 1 keeps this small:
// only broker credentials are needed, and they come from environment
// variables so users register their own Kite Connect app and keep the
// secret out of disk. A YAML config loader is deferred to a later sprint.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
)

// DefaultKiteRedirectPort is the localhost port used for the OAuth
// callback when KITE_REDIRECT_PORT is not set. Users must register
// http://127.0.0.1:7878/lakshmi/callback as the redirect URL in their
// Kite Connect app for the flow to work.
const DefaultKiteRedirectPort = 7878

// Broker holds the credentials and settings required to drive the
// brokerage login flow. Only Zerodha is supported in Sprint 1.
type Broker struct {
	APIKey       string
	APISecret    string
	RedirectPort int
}

// LoadBroker reads broker credentials from the environment. API_KEY is
// the only field marked "public"; API_SECRET must never be logged or
// written to disk.
//
// Required env vars:
//
//	KITE_API_KEY     — public Kite Connect API key
//	KITE_API_SECRET  — Kite Connect API secret
//
// Optional:
//
//	KITE_REDIRECT_PORT  — localhost callback port (default 7878)
func LoadBroker() (Broker, error) {
	b := Broker{
		APIKey:       os.Getenv("KITE_API_KEY"),
		APISecret:    os.Getenv("KITE_API_SECRET"),
		RedirectPort: DefaultKiteRedirectPort,
	}
	if v := os.Getenv("KITE_REDIRECT_PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil || p <= 0 || p > 65535 {
			return b, fmt.Errorf("KITE_REDIRECT_PORT: %q is not a valid port", v)
		}
		b.RedirectPort = p
	}
	if b.APIKey == "" {
		return b, errors.New("KITE_API_KEY is not set — create a Kite Connect app at https://developers.kite.trade and export the key")
	}
	if b.APISecret == "" {
		return b, errors.New("KITE_API_SECRET is not set — export the secret shown in your Kite Connect app")
	}
	return b, nil
}

// RedirectURL is the full OAuth redirect URL that must match the app
// config on https://developers.kite.trade.
func (b Broker) RedirectURL() string {
	return fmt.Sprintf("http://127.0.0.1:%d/lakshmi/callback", b.RedirectPort)
}
