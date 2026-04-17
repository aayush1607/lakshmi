// Package paths resolves on-disk locations used by Lakshmi.
//
// All user state lives under a single root directory (default: ~/.lakshmi),
// which can be overridden for tests or alternative installs via the
// LAKSHMI_HOME environment variable.
package paths

import (
	"os"
	"path/filepath"
)

// EnvHome is the environment variable that, when set, overrides the default
// root directory. This is useful for tests and for users who want to keep
// Lakshmi's state somewhere other than $HOME.
const EnvHome = "LAKSHMI_HOME"

// Home returns the root directory for Lakshmi state.
// It honours $LAKSHMI_HOME if set, otherwise resolves to ~/.lakshmi.
func Home() string {
	if v := os.Getenv(EnvHome); v != "" {
		return v
	}
	h, err := os.UserHomeDir()
	if err != nil {
		// Extremely unlikely; fall back to cwd-relative so we never panic.
		return ".lakshmi"
	}
	return filepath.Join(h, ".lakshmi")
}

// Data returns the directory used for cached market data.
func Data() string { return filepath.Join(Home(), "data") }

// Logs returns the directory used for rolling log files.
func Logs() string { return filepath.Join(Home(), "logs") }

// HistoryFile returns the path to the REPL command history file.
func HistoryFile() string { return filepath.Join(Home(), "history") }

// ConfigFile returns the path to the user's YAML config.
func ConfigFile() string { return filepath.Join(Home(), "config.yaml") }

// SessionFile returns the path to the persisted broker session metadata.
// The access token does NOT live here — only non-secret fields (user id,
// expiry, fetched_at). Secrets belong in the OS keychain.
func SessionFile() string { return filepath.Join(Home(), "session.json") }

// EnsureHome creates the root directory (and common subdirectories) with
// restrictive permissions (0700) if it does not already exist. Safe to call
// on every startup; it is a no-op when the directories already exist.
func EnsureHome() error {
	for _, dir := range []string{Home(), Data(), Logs()} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	return nil
}
