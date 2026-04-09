package app

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/sebastian/jira-cli/internal/config"
)

func TestLsCmdRunPrintsSprintHeaderBeforeTickets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/rest/agile/1.0/board":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"isLast":true,"values":[{"id":17,"name":"Backend Board"}]}`))
		case r.URL.Path == "/rest/agile/1.0/board/17/sprint":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"isLast":true,"values":[{"id":23,"name":"Sprint Alpha"}]}`))
		case r.URL.Path == "/rest/agile/1.0/board/17/sprint/23/issue":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"total":1,"issues":[{"key":"PROJ-1","fields":{"summary":"Implement ls","status":{"name":"In Progress"}}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ctx := &Context{CLI: &CLI{
		BaseURL: server.URL,
		Token:   "token",
		Cfg: config.Config{
			Project: "PROJ",
			BoardID: 17,
			BoardByProject: map[string]int{
				"PROJ": 17,
			},
		},
	}}

	output := captureStdout(t, func() {
		if err := (&LsCmd{Sprint: "Sprint Alpha"}).Run(ctx); err != nil {
			t.Fatal(err)
		}
	})

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected sprint header and ticket output, got %q", output)
	}
	if lines[0] != "Sprint Alpha (23)" {
		t.Fatalf("expected sprint header first, got %q", lines[0])
	}
	if lines[1] != "PROJ-1 Implement ls" {
		t.Fatalf("expected ticket line second, got %q", lines[1])
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	os.Stdout = w
	defer func() { os.Stdout = orig }()

	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	fn()

	_ = w.Close()
	return <-done
}
