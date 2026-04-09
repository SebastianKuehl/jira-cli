package app

import (
	"bufio"
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebastian/jira-cli/internal/config"
	"github.com/sebastian/jira-cli/internal/jira"
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

func TestResolveSprintSelectionPromptsForAmbiguousNumericFragment(t *testing.T) {
	sprints := []jira.Sprint{
		{ID: 23, Name: "Sprint 120 Alpha"},
		{ID: 24, Name: "Sprint 201 Beta"},
		{ID: 25, Name: "Sprint 120.1 Gamma"},
	}

	var out bytes.Buffer
	selected, err := resolveSprintSelection(sprints, "20", bufio.NewReader(strings.NewReader("gamma\n1\n")), &out, true)
	if err != nil {
		t.Fatal(err)
	}
	if selected == nil || selected.ID != 25 {
		t.Fatalf("expected Sprint 120.1 Gamma, got %#v", selected)
	}
	if !strings.Contains(out.String(), `Sprint "20" matches multiple sprints:`) {
		t.Fatalf("expected ambiguity prompt, got %q", out.String())
	}
}

func TestResolveSprintSelectionReturnsAmbiguityWhenNonInteractive(t *testing.T) {
	sprints := []jira.Sprint{
		{ID: 23, Name: "Sprint 120 Alpha"},
		{ID: 24, Name: "Sprint 201 Beta"},
	}

	selected, err := resolveSprintSelection(sprints, "20", bufio.NewReader(strings.NewReader("")), &bytes.Buffer{}, false)
	if err == nil {
		t.Fatalf("expected ambiguity error, got %#v", selected)
	}
	if !strings.Contains(err.Error(), `sprint "20" is ambiguous`) {
		t.Fatalf("expected ambiguity error, got %v", err)
	}
}

func TestResolveSprintSelectionFindsSingleApproximateSprint(t *testing.T) {
	sprints := []jira.Sprint{
		{ID: 23, Name: "Sprint 120 Alpha"},
		{ID: 24, Name: "Sprint 301 Beta"},
	}

	selected, err := resolveSprintSelection(sprints, "20", bufio.NewReader(strings.NewReader("")), &bytes.Buffer{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if selected == nil || selected.ID != 23 {
		t.Fatalf("expected Sprint 120 Alpha, got %#v", selected)
	}
}

func TestLsCmdRunMarksLocalSprintsInList(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/rest/agile/1.0/board":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"isLast":true,"values":[{"id":17,"name":"Backend Board"}]}`))
		case r.URL.Path == "/rest/agile/1.0/board/17/sprint":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"isLast":true,"values":[{"id":23,"name":"Sprint Alpha"},{"id":24,"name":"Sprint Beta"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "Sprint Alpha"), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx := &Context{CLI: &CLI{
		BaseURL: server.URL,
		Token:   "token",
		Cfg: config.Config{
			Project:  "PROJ",
			BasePath: root,
			BoardID:  17,
			BoardByProject: map[string]int{
				"PROJ": 17,
			},
		},
	}}

	output := captureStdout(t, func() {
		if err := (&LsCmd{}).Run(ctx); err != nil {
			t.Fatal(err)
		}
	})

	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected two sprint lines, got %q", output)
	}
	if lines[0] != "Sprint Alpha ✅ local" {
		t.Fatalf("expected local sprint marker, got %q", lines[0])
	}
	if lines[1] != "Sprint Beta" {
		t.Fatalf("expected unmarked remote sprint, got %q", lines[1])
	}
}

func TestLsCmdRunUsesCachedOutput(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("JIRA_CONFIG_DIR", cacheDir)

	key := cacheKey("ls", "200")
	if err := writeCommandCache(ctxForCacheTest(), key, "cached ls output\n"); err != nil {
		t.Fatal(err)
	}

	ctx := ctxForCacheTest()
	output := captureStdout(t, func() {
		if err := (&LsCmd{Sprint: "200"}).Run(ctx); err != nil {
			t.Fatal(err)
		}
	})

	if !strings.Contains(output, "cached ls output\n") {
		t.Fatalf("expected cached ls output, got %q", output)
	}
	if !strings.Contains(output, "⚠️ cache hit: ls|200") {
		t.Fatalf("expected cache note, got %q", output)
	}
}

func TestLsCmdRunUpdateCacheOverridesCachedOutput(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("JIRA_CONFIG_DIR", cacheDir)
	if err := writeCommandCache(ctxForCacheTest(), cacheKey("ls", "Sprint Alpha"), "stale\n"); err != nil {
		t.Fatal(err)
	}

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
			_, _ = w.Write([]byte(`{"total":1,"issues":[{"key":"PROJ-1","fields":{"summary":"fresh","status":{"name":"Todo"}}}]}`))
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
		if err := (&LsCmd{Sprint: "Sprint Alpha", UpdateCache: true}).Run(ctx); err != nil {
			t.Fatal(err)
		}
	})

	if strings.Contains(output, "stale") {
		t.Fatalf("expected fresh output, got %q", output)
	}
	cached, ok, err := readCommandCache(ctx, cacheKey("ls", "Sprint Alpha"))
	if err != nil {
		t.Fatal(err)
	}
	if !ok || !strings.Contains(cached, "Sprint Alpha (23)") {
		t.Fatalf("expected refreshed cache, got ok=%v cached=%q", ok, cached)
	}
}

func TestLsCmdRunCachedSprintListReappliesLocalMarker(t *testing.T) {
	cacheDir := t.TempDir()
	root := t.TempDir()
	t.Setenv("JIRA_CONFIG_DIR", cacheDir)

	ctx := &Context{CLI: &CLI{
		BaseURL: "https://example.atlassian.net",
		Cfg: config.Config{
			Project:  "PROJ",
			BasePath: root,
			BoardID:  17,
		},
	}}
	if err := writeCommandCache(ctx, cacheKey("ls", ""), "Sprint Alpha\nSprint Beta\n"); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "Sprint Beta"), 0o755); err != nil {
		t.Fatal(err)
	}

	output := captureStdout(t, func() {
		if err := (&LsCmd{}).Run(ctx); err != nil {
			t.Fatal(err)
		}
	})

	if !strings.Contains(output, "Sprint Beta ✅ local") {
		t.Fatalf("expected cached output to reapply local marker, got %q", output)
	}
	if strings.Contains(output, "Sprint Alpha ✅ local") {
		t.Fatalf("expected non-local sprint to stay unmarked, got %q", output)
	}
}

func ctxForCacheTest() *Context {
	return &Context{CLI: &CLI{
		BaseURL: "https://example.atlassian.net",
		Cfg: config.Config{
			Project: "PROJ",
			BoardID: 17,
		},
	}}
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
