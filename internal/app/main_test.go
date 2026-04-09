package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		panic(err)
	}
	cacheRoot := filepath.Join(cacheDir, "jira-cli")
	if err := os.MkdirAll(cacheRoot, 0o755); err != nil {
		panic(err)
	}
	configDir, err := os.MkdirTemp(cacheRoot, "test-config-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(configDir)

	if err := os.Setenv("JIRA_CONFIG_DIR", configDir); err != nil {
		panic(err)
	}

	code := m.Run()
	_ = os.RemoveAll(configDir)
	os.Exit(code)
}
