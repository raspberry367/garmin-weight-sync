package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewConfigLoadsValuesFromDotEnvFile(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	if err := os.WriteFile(envPath, []byte("SERVER_PORT=4000\nGARMIN_USERNAME=demo-user\nGARMIN_PASSWORD=demo-pass\nAPI_KEY=demo-key\n"), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldWd)
	}()

	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	t.Setenv("SERVER_PORT", "")
	t.Setenv("GARMIN_USERNAME", "")
	t.Setenv("GARMIN_PASSWORD", "")
	t.Setenv("API_KEY", "")

	cfg, err := NewConfig()
	if err != nil {
		t.Fatalf("NewConfig returned error: %v", err)
	}

	if cfg.ServerPort != "4000" {
		t.Fatalf("expected ServerPort to be loaded from .env, got %q", cfg.ServerPort)
	}
	if cfg.GarminUsername != "demo-user" {
		t.Fatalf("expected Garmin username to be loaded from .env, got %q", cfg.GarminUsername)
	}
	if cfg.GarminPassword != "demo-pass" {
		t.Fatalf("expected Garmin password to be loaded from .env, got %q", cfg.GarminPassword)
	}
	if cfg.APIKey != "demo-key" {
		t.Fatalf("expected API key to be loaded from .env, got %q", cfg.APIKey)
	}
}
