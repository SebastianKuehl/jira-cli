package config

import (
	"path/filepath"
	"testing"
)

func TestPathUsesExplicitConfigPathEnv(t *testing.T) {
	t.Setenv(configPathEnv, filepath.Join(t.TempDir(), "custom-config.json"))
	t.Setenv(configDirEnv, filepath.Join(t.TempDir(), "ignored"))

	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "custom-config.json" {
		t.Fatalf("expected explicit config path, got %q", path)
	}
}

func TestPathUsesConfigDirEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(configDirEnv, dir)
	t.Setenv(configPathEnv, "")

	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(dir, fileName) {
		t.Fatalf("expected config dir override, got %q", path)
	}
}

func TestCacheDirUsesConfigDirEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(configDirEnv, dir)
	t.Setenv(configPathEnv, "")

	cacheDir, err := CacheDir()
	if err != nil {
		t.Fatal(err)
	}
	if cacheDir != filepath.Join(dir, cacheDirName) {
		t.Fatalf("expected cache dir under config dir, got %q", cacheDir)
	}
}
