package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTemp(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadRejectsDuplicateProviders(t *testing.T) {
	dir := t.TempDir()
	cfg := writeTemp(t, dir, "config.yml", "model: a/x\n")
	providers := writeTemp(t, dir, "providers.yml", "- name: a\n  url: https://example.com\n  key: k\n- name: a\n  url: https://example.org\n  key: k\n")
	if _, err := Load(cfg, providers); err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadRejectsUnknownFieldsAndBadURL(t *testing.T) {
	dir := t.TempDir()
	cfg := writeTemp(t, dir, "config.yml", "model: a/x\nextra: 1\n")
	providers := writeTemp(t, dir, "providers.yml", "- name: a\n  url: ://bad\n  key: k\n")
	if _, err := Load(cfg, providers); err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadAppliesDefaultsAndRejectsBadBounds(t *testing.T) {
	dir := t.TempDir()
	cfg := writeTemp(t, dir, "config.yml", "model: a/x\nmax_turns: -1\n")
	providers := writeTemp(t, dir, "providers.yml", "- name: a\n  url: https://example.com\n  key: k\n")
	if _, err := Load(cfg, providers); err == nil {
		t.Fatal("expected error")
	}

	cfg = writeTemp(t, dir, "config.yml", "model: a/x\n")
	loaded, err := Load(cfg, providers)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.MaxTokens != 4096 || loaded.MaxTurns != 50 {
		t.Fatalf("unexpected defaults: %+v", loaded)
	}
}

func TestLoadOnlyExpandsEnvForSelectedProvider(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("USED_KEY", "secret")

	cfg := writeTemp(t, dir, "config.yml", "model: used/x\n")
	providers := writeTemp(t, dir, "providers.yml", "- name: used\n  url: https://example.com\n  key: ${USED_KEY}\n- name: unused\n  url: https://unused.example.com\n  key: ${MISSING_KEY}\n")

	loaded, err := Load(cfg, providers)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if loaded.APIKey != "secret" {
		t.Fatalf("expected selected provider key to resolve, got %q", loaded.APIKey)
	}
}
