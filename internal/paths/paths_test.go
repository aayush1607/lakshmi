package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHomeHonoursEnv(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(EnvHome, tmp)
	if got := Home(); got != tmp {
		t.Fatalf("Home() = %q, want %q", got, tmp)
	}
}

func TestSubdirectoriesDeriveFromHome(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(EnvHome, tmp)

	cases := map[string]string{
		"Data":        filepath.Join(tmp, "data"),
		"Logs":        filepath.Join(tmp, "logs"),
		"HistoryFile": filepath.Join(tmp, "history"),
		"ConfigFile":  filepath.Join(tmp, "config.yaml"),
	}
	got := map[string]string{
		"Data":        Data(),
		"Logs":        Logs(),
		"HistoryFile": HistoryFile(),
		"ConfigFile":  ConfigFile(),
	}
	for k, want := range cases {
		if got[k] != want {
			t.Errorf("%s = %q, want %q", k, got[k], want)
		}
	}
}

func TestEnsureHomeCreatesDirectories(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(EnvHome, filepath.Join(tmp, "nested", ".lakshmi"))

	if err := EnsureHome(); err != nil {
		t.Fatalf("EnsureHome: %v", err)
	}
	for _, d := range []string{Home(), Data(), Logs()} {
		info, err := os.Stat(d)
		if err != nil {
			t.Fatalf("stat %s: %v", d, err)
		}
		if !info.IsDir() {
			t.Errorf("%s is not a directory", d)
		}
	}
}
