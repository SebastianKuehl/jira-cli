package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sebastian/jira-cli/internal/config"
)

func TestCatCmdRunPrintsTicketWithUpdateCache(t *testing.T) {
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
	if !strings.Contains(output, "title:    Fresh title") {
		t.Fatalf("expected rendered ticket output, got %q", output)
	}
}
