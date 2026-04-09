package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sebastian/jira-cli/internal/config"
)

func TestCatCmdRunUsesCachedOutput(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("JIRA_CONFIG_DIR", cacheDir)

	key := cacheKey("cat", "FAS-123456")
	if err := writeCommandCache(ctxForCacheTest(), key, "cached cat output\n"); err != nil {
		t.Fatal(err)
	}

	ctx := ctxForCacheTest()
	output := captureStdout(t, func() {
		if err := (&CatCmd{Target: "FAS-123456"}).Run(ctx); err != nil {
			t.Fatal(err)
		}
	})

	if !strings.Contains(output, "cached cat output\n") {
		t.Fatalf("expected cached cat output, got %q", output)
	}
	if !strings.Contains(output, "⚠️ cache hit: cat|FAS-123456") {
		t.Fatalf("expected cache note, got %q", output)
	}
}

func TestCatCmdRunUpdateCacheOverridesCachedOutput(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("JIRA_CONFIG_DIR", cacheDir)
	if err := writeCommandCache(ctxForCacheTest(), cacheKey("cat", "PROJ-1"), "stale\n"); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/api/2/issue/PROJ-1":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"key":"PROJ-1","fields":{"summary":"Fresh title","description":"Fresh body","status":{"name":"Todo"}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ctx := &Context{CLI: &CLI{
		BaseURL: server.URL,
		Token:   "token",
		Cfg:     config.Config{},
	}}

	output := captureStdout(t, func() {
		if err := (&CatCmd{Target: "PROJ-1", UpdateCache: true}).Run(ctx); err != nil {
			t.Fatal(err)
		}
	})

	if strings.Contains(output, "stale") {
		t.Fatalf("expected fresh output, got %q", output)
	}
	cached, ok, err := readCommandCache(ctx, cacheKey("cat", "PROJ-1"))
	if err != nil {
		t.Fatal(err)
	}
	if !ok || !strings.Contains(cached, "Fresh title") {
		t.Fatalf("expected refreshed cat cache, got ok=%v cached=%q", ok, cached)
	}
}
