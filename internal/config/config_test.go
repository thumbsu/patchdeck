package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndLoad(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("PATCHDECK_CONFIG_HOME", tmp)

	cfg := Config{DefaultRepo: "/tmp/repo"}
	if err := Save(cfg); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.DefaultRepo != cfg.DefaultRepo {
		t.Fatalf("expected %q, got %q", cfg.DefaultRepo, loaded.DefaultRepo)
	}

	path := filepath.Join(tmp, "patchdeck", "config.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected config file to exist: %v", err)
	}
}
