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
	"sync/atomic"
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

func TestLsCmdRunVerbosePrintsAssigneeAndReporterOnSeparateLines(t *testing.T) {
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
			_, _ = w.Write([]byte(`{"total":1,"issues":[{"key":"PROJ-1","fields":{"summary":"Implement ls","status":{"name":"In Progress"},"assignee":{"displayName":"Alice"},"reporter":{"displayName":"Bob"}}}]}`))
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
		if err := (&LsCmd{Sprint: "Sprint Alpha", Verbose: true, UpdateCache: true}).Run(ctx); err != nil {
			t.Fatal(err)
		}
	})

	if !strings.Contains(output, "  assignee: Alice\n  reporter: Bob\n  state: In Progress\n") {
		t.Fatalf("expected assignee/reporter/state on separate lines, got %q", output)
	}
	if strings.Contains(output, "assignee: Alice | reporter: Bob") {
		t.Fatalf("expected combined assignee/reporter line removed, got %q", output)
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

func TestResolveSprintSelectionAcceptsDisplayedSprintFormat(t *testing.T) {
	sprints := []jira.Sprint{
		{ID: 23, Name: "Sprint Alpha"},
		{ID: 24, Name: "Sprint Beta"},
	}

	selected, err := resolveSprintSelection(sprints, "Sprint Alpha (23)", bufio.NewReader(strings.NewReader("")), &bytes.Buffer{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if selected == nil || selected.ID != 23 {
		t.Fatalf("expected Sprint Alpha by displayed format, got %#v", selected)
	}
}

func TestResolveSprintSelectionPreservesExactNameWithParenthesizedDigits(t *testing.T) {
	sprints := []jira.Sprint{
		{ID: 23, Name: "Release (2024)"},
		{ID: 2024, Name: "Different Sprint"},
	}

	selected, err := resolveSprintSelection(sprints, "Release (2024)", bufio.NewReader(strings.NewReader("")), &bytes.Buffer{}, false)
	if err != nil {
		t.Fatal(err)
	}
	if selected == nil || selected.ID != 23 {
		t.Fatalf("expected exact sprint-name match to win, got %#v", selected)
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
	if lines[0] != "Sprint Alpha (23) ✅ local" {
		t.Fatalf("expected local sprint marker, got %q", lines[0])
	}
	if lines[1] != "Sprint Beta (24)" {
		t.Fatalf("expected unmarked remote sprint, got %q", lines[1])
	}
}

func TestEnsureBoardUsesConfiguredBoardWithoutListingBoards(t *testing.T) {
	var boardRequests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rest/agile/1.0/board" {
			atomic.AddInt32(&boardRequests, 1)
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	ctx := &Context{CLI: &CLI{
		BaseURL: server.URL,
		Token:   "token",
		Cfg: config.Config{
			Project: "PROJ",
			BoardByProject: map[string]int{
				"PROJ": 17,
			},
		},
	}}

	client, err := ctx.JiraClient()
	if err != nil {
		t.Fatal(err)
	}
	boardID, err := ensureBoard(ctx, client, "PROJ")
	if err != nil {
		t.Fatal(err)
	}
	if boardID != 17 {
		t.Fatalf("expected configured board id 17, got %d", boardID)
	}
	if got := atomic.LoadInt32(&boardRequests); got != 0 {
		t.Fatalf("expected no board list request, got %d", got)
	}
}

func TestResolveBoardSprintsClearsStaleConfiguredBoardOnNotFound(t *testing.T) {
	var boardRequests int32
	var sprintRequests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/rest/agile/1.0/board/17/sprint":
			atomic.AddInt32(&sprintRequests, 1)
			http.NotFound(w, r)
		case r.URL.Path == "/rest/agile/1.0/board":
			atomic.AddInt32(&boardRequests, 1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"isLast":true,"values":[{"id":19,"name":"Fallback Board"}]}`))
		case r.URL.Path == "/rest/agile/1.0/board/19/sprint":
			atomic.AddInt32(&sprintRequests, 1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"isLast":true,"values":[{"id":23,"name":"Sprint Alpha"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfgDir := t.TempDir()
	t.Setenv("JIRA_CONFIG_DIR", cfgDir)
	ctx := &Context{CLI: &CLI{
		BaseURL: server.URL,
		Token:   "token",
		Cfg: config.Config{
			Project: "PROJ",
			BoardID: 19,
			BoardByProject: map[string]int{
				"PROJ": 17,
			},
		},
	}}

	client, err := ctx.JiraClient()
	if err != nil {
		t.Fatal(err)
	}
	boardID, sprints, err := resolveBoardSprints(ctx, client, "PROJ", 17)
	if err != nil {
		t.Fatal(err)
	}
	if boardID != 19 {
		t.Fatalf("expected fallback board id 19, got %d", boardID)
	}
	if len(sprints) != 1 || sprints[0].Name != "Sprint Alpha" {
		t.Fatalf("expected fallback sprint list, got %#v", sprints)
	}
	if got := atomic.LoadInt32(&boardRequests); got != 0 {
		t.Fatalf("expected no board list request when fallback BoardID is configured, got %d", got)
	}
	if got := atomic.LoadInt32(&sprintRequests); got != 2 {
		t.Fatalf("expected two sprint requests (stale + retry), got %d", got)
	}
	if ctx.CLI.Cfg.BoardByProject["PROJ"] != 19 {
		t.Fatalf("expected board mapping updated to fallback board, got %#v", ctx.CLI.Cfg.BoardByProject)
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
