package app

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sebastian/jira-cli/internal/config"
	"github.com/sebastian/jira-cli/internal/jira"
)

func TestFetchCmdRunFetchesSprintTicketsToMarkdown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/rest/agile/1.0/board":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"isLast":true,"values":[{"id":17,"name":"Backend Board"}]}`))
		case r.URL.Path == "/rest/agile/1.0/board/17/sprint":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"isLast":true,"values":[{"id":23,"name":"Sprint / Alpha","goal":"Ship it"}]}`))
		case r.URL.Path == "/rest/agile/1.0/board/17/sprint/23/issue":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"total":1,"issues":[{"key":"PROJ-1","fields":{"summary":"Implement fetch","status":{"name":"In Progress"},"assignee":{"displayName":"Alice"},"reporter":{"displayName":"Bob"}}}]}`))
		case r.URL.Path == "/rest/api/2/issue/PROJ-1":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"key":"PROJ-1","fields":{"summary":"Implement fetch","description":"Ticket body","priority":{"name":"High"},"labels":["cli","sync"],"status":{"name":"In Progress"},"assignee":{"displayName":"Alice"},"reporter":{"displayName":"Bob"}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	root := t.TempDir()
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
			BoardNameByProject: map[string]string{
				"PROJ": "Backend Board",
			},
		},
	}}

	cmd := &FetchCmd{}
	if err := cmd.Run(ctx); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(root, "Sprint - Alpha", "PROJ-1.md")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(body)
	for _, want := range []string{
		"id: PROJ-1",
		"title: Implement fetch",
		"workflow_state: In Progress",
		"priority: High",
		"labels: cli, sync",
		"url: " + server.URL + "/browse/PROJ-1",
		"Ticket body",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("expected %q in markdown, got:\n%s", want, content)
		}
	}
}

func TestFetchCmdRunFetchesTicketIntoMatchingSprint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/rest/agile/1.0/board":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"isLast":true,"values":[{"id":17,"name":"Backend Board"}]}`))
		case r.URL.Path == "/rest/agile/1.0/board/17/sprint":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"isLast":true,"values":[{"id":23,"name":"Sprint A"},{"id":24,"name":"Sprint B"}]}`))
		case r.URL.Path == "/rest/agile/1.0/board/17/sprint/23/issue":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"total":0,"issues":[]}`))
		case r.URL.Path == "/rest/agile/1.0/board/17/sprint/24/issue":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"total":1,"issues":[{"key":"PROJ-2","fields":{"summary":"Fix fetch","status":{"name":"Todo"}}}]}`))
		case r.URL.Path == "/rest/api/2/issue/PROJ-2":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"key":"PROJ-2","fields":{"summary":"Fix fetch","description":"Specific ticket body","labels":[],"status":{"name":"Todo"}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	root := t.TempDir()
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

	cmd := &FetchCmd{Target: "PROJ-2"}
	if err := cmd.Run(ctx); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(root, "Sprint B", "PROJ-2.md")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "Specific ticket body") {
		t.Fatalf("expected fetched ticket body, got:\n%s", string(body))
	}
}

func TestFetchCmdRunFetchesLowercaseTicketTarget(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/rest/agile/1.0/board":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"isLast":true,"values":[{"id":17,"name":"Backend Board"}]}`))
		case r.URL.Path == "/rest/agile/1.0/board/17/sprint":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"isLast":true,"values":[{"id":24,"name":"Sprint B"}]}`))
		case r.URL.Path == "/rest/agile/1.0/board/17/sprint/24/issue":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"total":1,"issues":[{"key":"PROJ-2","fields":{"summary":"Fix fetch","status":{"name":"Todo"}}}]}`))
		case r.URL.Path == "/rest/api/2/issue/PROJ-2":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"key":"PROJ-2","fields":{"summary":"Fix fetch","description":"Specific ticket body","labels":[],"status":{"name":"Todo"}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	root := t.TempDir()
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

	cmd := &FetchCmd{Target: "proj-2"}
	if err := cmd.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "Sprint B", "PROJ-2.md")); err != nil {
		t.Fatal(err)
	}
}

func TestFetchCmdRunFetchesSprintByPositionalTarget(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/rest/agile/1.0/board":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"isLast":true,"values":[{"id":17,"name":"Backend Board"}]}`))
		case r.URL.Path == "/rest/agile/1.0/board/17/sprint":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"isLast":true,"values":[{"id":23,"name":"Sprint A"},{"id":24,"name":"Sprint B"}]}`))
		case r.URL.Path == "/rest/agile/1.0/board/17/sprint/23/issue":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"total":1,"issues":[{"key":"PROJ-5","fields":{"summary":"Sprint specific","status":{"name":"Todo"}}}]}`))
		case r.URL.Path == "/rest/agile/1.0/board/17/sprint/24/issue":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"total":1,"issues":[{"key":"PROJ-6","fields":{"summary":"Wrong sprint","status":{"name":"Todo"}}}]}`))
		case r.URL.Path == "/rest/api/2/issue/PROJ-5":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"key":"PROJ-5","fields":{"summary":"Sprint specific","description":"Only sprint A","labels":[],"status":{"name":"Todo"}}}`))
		case r.URL.Path == "/rest/api/2/issue/PROJ-6":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"key":"PROJ-6","fields":{"summary":"Wrong sprint","description":"Should not be fetched","labels":[],"status":{"name":"Todo"}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	root := t.TempDir()
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

	cmd := &FetchCmd{Target: "Sprint A"}
	if err := cmd.Run(ctx); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(root, "Sprint A", "PROJ-5.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "Sprint B", "PROJ-6.md")); !os.IsNotExist(err) {
		t.Fatalf("expected Sprint B ticket to remain unfetched, stat err=%v", err)
	}
}

func TestFetchCmdRunFiltersSprintsByYear(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/rest/agile/1.0/board":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"isLast":true,"values":[{"id":17,"name":"Backend Board"}]}`))
		case r.URL.Path == "/rest/agile/1.0/board/17/sprint":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"isLast":true,"values":[{"id":23,"name":"Sprint 2026","startDate":"2026-02-01T00:00:00.000Z"},{"id":24,"name":"Sprint 2025","startDate":"2025-12-01T00:00:00.000Z"}]}`))
		case r.URL.Path == "/rest/agile/1.0/board/17/sprint/23/issue":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"total":1,"issues":[{"key":"PROJ-26","fields":{"summary":"Current year","status":{"name":"Todo"}}}]}`))
		case r.URL.Path == "/rest/agile/1.0/board/17/sprint/24/issue":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"total":1,"issues":[{"key":"PROJ-25","fields":{"summary":"Old year","status":{"name":"Todo"}}}]}`))
		case r.URL.Path == "/rest/api/2/issue/PROJ-26":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"key":"PROJ-26","fields":{"summary":"Current year","description":"Fetched","labels":[],"status":{"name":"Todo"}}}`))
		case r.URL.Path == "/rest/api/2/issue/PROJ-25":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"key":"PROJ-25","fields":{"summary":"Old year","description":"Should not fetch","labels":[],"status":{"name":"Todo"}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	root := t.TempDir()
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

	cmd := &FetchCmd{Year: 2026}
	if err := cmd.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "Sprint 2026", "PROJ-26.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "Sprint 2025", "PROJ-25.md")); !os.IsNotExist(err) {
		t.Fatalf("expected non-matching year sprint to be skipped, stat err=%v", err)
	}
}

func TestFetchCmdRunTicketRespectsYearFilter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/rest/agile/1.0/board":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"isLast":true,"values":[{"id":17,"name":"Backend Board"}]}`))
		case r.URL.Path == "/rest/agile/1.0/board/17/sprint":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"isLast":true,"values":[{"id":24,"name":"Sprint 2025","startDate":"2025-12-01T00:00:00.000Z"}]}`))
		case r.URL.Path == "/rest/agile/1.0/board/17/sprint/24/issue":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"total":1,"issues":[{"key":"PROJ-25","fields":{"summary":"Old year","status":{"name":"Todo"}}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	root := t.TempDir()
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

	err := (&FetchCmd{Target: "PROJ-25", Year: 2026}).Run(ctx)
	if err == nil || !strings.Contains(err.Error(), "no sprints found for year 2026") {
		t.Fatalf("expected year filter error, got %v", err)
	}
}

func TestFetchCmdRunRejectsInvalidYear(t *testing.T) {
	err := (&FetchCmd{Year: 26}).Run(&Context{CLI: &CLI{}})
	if err == nil || !strings.Contains(err.Error(), "four digit year") {
		t.Fatalf("expected invalid year error, got %v", err)
	}
}

func TestFetchCmdRunApproximatesSprintTarget(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/rest/agile/1.0/board":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"isLast":true,"values":[{"id":17,"name":"Backend Board"}]}`))
		case r.URL.Path == "/rest/agile/1.0/board/17/sprint":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"isLast":true,"values":[{"id":201,"name":"E51(S4).DevS201","startDate":"2026-04-01T00:00:00.000Z"},{"id":202,"name":"E51(S4).DevS202","startDate":"2026-03-01T00:00:00.000Z"}]}`))
		case r.URL.Path == "/rest/agile/1.0/board/17/sprint/201/issue":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"total":1,"issues":[{"key":"PROJ-201","fields":{"summary":"Approx sprint","status":{"name":"Todo"}}}]}`))
		case r.URL.Path == "/rest/agile/1.0/board/17/sprint/202/issue":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"total":1,"issues":[{"key":"PROJ-202","fields":{"summary":"Wrong sprint","status":{"name":"Todo"}}}]}`))
		case r.URL.Path == "/rest/api/2/issue/PROJ-201":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"key":"PROJ-201","fields":{"summary":"Approx sprint","description":"Matched via 201","labels":[],"status":{"name":"Todo"}}}`))
		case r.URL.Path == "/rest/api/2/issue/PROJ-202":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"key":"PROJ-202","fields":{"summary":"Wrong sprint","description":"Should not be fetched","labels":[],"status":{"name":"Todo"}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	root := t.TempDir()
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

	cmd := &FetchCmd{Target: "201"}
	if err := cmd.Run(ctx); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(root, "E51(S4).DevS201", "PROJ-201.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "E51(S4).DevS202", "PROJ-202.md")); !os.IsNotExist(err) {
		t.Fatalf("expected approximate match to stay on the 201 sprint, stat err=%v", err)
	}
}

func TestFetchCmdRunPromptsForAmbiguousSprintTarget(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/rest/agile/1.0/board":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"isLast":true,"values":[{"id":17,"name":"Backend Board"}]}`))
		case r.URL.Path == "/rest/agile/1.0/board/17/sprint":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"isLast":true,"values":[{"id":120,"name":"Sprint 120"},{"id":201,"name":"Sprint 201"},{"id":1201,"name":"Sprint 120.1"}]}`))
		case r.URL.Path == "/rest/agile/1.0/board/17/sprint/1201/issue":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"total":1,"issues":[{"key":"PROJ-1201","fields":{"summary":"Chosen sprint","status":{"name":"Todo"}}}]}`))
		case r.URL.Path == "/rest/agile/1.0/board/17/sprint/120/issue":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"total":1,"issues":[{"key":"PROJ-120","fields":{"summary":"Wrong sprint","status":{"name":"Todo"}}}]}`))
		case r.URL.Path == "/rest/agile/1.0/board/17/sprint/201/issue":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"total":1,"issues":[{"key":"PROJ-201","fields":{"summary":"Wrong sprint","status":{"name":"Todo"}}}]}`))
		case r.URL.Path == "/rest/api/2/issue/PROJ-1201":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"key":"PROJ-1201","fields":{"summary":"Chosen sprint","description":"Picked interactively","labels":[],"status":{"name":"Todo"}}}`))
		case r.URL.Path == "/rest/api/2/issue/PROJ-120":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"key":"PROJ-120","fields":{"summary":"Wrong sprint","description":"Should not fetch","labels":[],"status":{"name":"Todo"}}}`))
		case r.URL.Path == "/rest/api/2/issue/PROJ-201":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"key":"PROJ-201","fields":{"summary":"Wrong sprint","description":"Should not fetch","labels":[],"status":{"name":"Todo"}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	root := t.TempDir()
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

	origStdin := os.Stdin
	origStdout := os.Stdout
	origInteractive := stdinIsTerminalFunc
	inR, inW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer inR.Close()
	defer outR.Close()
	os.Stdin = inR
	os.Stdout = outW
	defer func() {
		os.Stdin = origStdin
		os.Stdout = origStdout
		stdinIsTerminalFunc = origInteractive
	}()
	stdinIsTerminalFunc = func() bool { return true }

	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, outR)
		done <- buf.String()
	}()

	if _, err := inW.WriteString("120.1\n1\n"); err != nil {
		t.Fatal(err)
	}
	_ = inW.Close()

	cmd := &FetchCmd{Target: "20"}
	if err := cmd.Run(ctx); err != nil {
		t.Fatal(err)
	}

	_ = outW.Close()
	output := <-done
	if !strings.Contains(output, `Sprint "20" matches multiple sprints:`) {
		t.Fatalf("expected prompt output, got %q", output)
	}
	if _, err := os.Stat(filepath.Join(root, "Sprint 120.1", "PROJ-1201.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "Sprint 120", "PROJ-120.md")); !os.IsNotExist(err) {
		t.Fatalf("expected Sprint 120 ticket to remain unfetched, stat err=%v", err)
	}
}

func TestSelectedSprintsRejectsAmbiguousApproximation(t *testing.T) {
	sprints := []jira.Sprint{
		{ID: 201, Name: "E51(S4).DevS201"},
		{ID: 202, Name: "E51(S4).DevS202"},
	}
	_, err := selectedSprints(sprints, "devs20")
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("expected ambiguous approximation error, got %v", err)
	}
}

func TestSelectedSprintsPrefersLatestTenForApproximation(t *testing.T) {
	sprints := []jira.Sprint{
		{ID: 201, Name: "E51(S4).DevS201", StartDate: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
	}
	for i := 0; i < 10; i++ {
		sprints = append(sprints, jira.Sprint{
			ID:        300 + i,
			Name:      fmt.Sprintf("Noise-%d", i),
			StartDate: time.Date(2026, 4, 20-i, 0, 0, 0, 0, time.UTC),
		})
	}
	sprints = append(sprints, jira.Sprint{ID: 1201, Name: "Legacy201", StartDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})

	selected, err := selectedSprints(sprints, "201")
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 1 || selected[0].Name != "E51(S4).DevS201" {
		t.Fatalf("expected latest ten window to win, got %#v", selected)
	}
}

func TestResolveFetchSprintSelectionPrefersLatestTenBeforePrompting(t *testing.T) {
	sprints := []jira.Sprint{
		{ID: 201, Name: "E51(S4).DevS201", StartDate: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
	}
	for i := 0; i < 10; i++ {
		sprints = append(sprints, jira.Sprint{
			ID:        300 + i,
			Name:      fmt.Sprintf("Noise-%d", i),
			StartDate: time.Date(2026, 4, 20-i, 0, 0, 0, 0, time.UTC),
		})
	}
	sprints = append(sprints, jira.Sprint{ID: 1201, Name: "Legacy201", StartDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})

	var out bytes.Buffer
	selected, err := resolveFetchSprintSelection(sprints, "201", bufio.NewReader(strings.NewReader("")), &out, true)
	if err != nil {
		t.Fatal(err)
	}
	if selected == nil || selected.ID != 201 {
		t.Fatalf("expected latest-ten sprint to win, got %#v", selected)
	}
	if out.Len() != 0 {
		t.Fatalf("expected no prompt output, got %q", out.String())
	}
}

func TestSelectedSprintsFallsBackToOlderApproximationWhenLatestTenMiss(t *testing.T) {
	var sprints []jira.Sprint
	for i := 0; i < 10; i++ {
		sprints = append(sprints, jira.Sprint{
			ID:        400 + i,
			Name:      fmt.Sprintf("Noise-%d", i),
			StartDate: time.Date(2026, 4, 20-i, 0, 0, 0, 0, time.UTC),
		})
	}
	sprints = append(sprints, jira.Sprint{ID: 999, Name: "Historic Sprint 201", StartDate: time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)})

	selected, err := selectedSprints(sprints, "201")
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) != 1 || selected[0].Name != "Historic Sprint 201" {
		t.Fatalf("expected fallback to older sprint when latest ten miss, got %#v", selected)
	}
}

func TestLatestSprintsUsesDatesNotListOrder(t *testing.T) {
	sprints := []jira.Sprint{
		{ID: 1, Name: "Older but first", StartDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		{ID: 2, Name: "Newer but second", StartDate: time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)},
	}

	got := latestSprints(sprints, 1)
	if len(got) != 1 || got[0].ID != 2 {
		t.Fatalf("expected newest sprint by date, got %#v", got)
	}
}

func TestFetchCmdRunRejectsAmbiguousPositionalTarget(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/rest/agile/1.0/board":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"isLast":true,"values":[{"id":17,"name":"Backend Board"}]}`))
		case r.URL.Path == "/rest/agile/1.0/board/17/sprint":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"isLast":true,"values":[{"id":23,"name":"PROJ-7"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	root := t.TempDir()
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

	err := (&FetchCmd{Target: "PROJ-7"}).Run(ctx)
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("expected ambiguity error, got %v", err)
	}
}

func TestSprintOutputDirRejectsTraversalNames(t *testing.T) {
	root := t.TempDir()
	dir, err := sprintOutputDir(root, jira.Sprint{ID: 9, Name: ".."})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(dir) != "Sprint-9" {
		t.Fatalf("expected safe fallback dir name Sprint-9, got %q", dir)
	}
	rootResolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(rootResolved, dir)
	if err != nil {
		t.Fatal(err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		t.Fatalf("expected dir %q to stay under %q", dir, rootResolved)
	}
}

func TestWriteFetchedTicketRejectsSymlinkSprintDir(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(root, "Sprint A")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink not supported on this system: %v", err)
	}
	err := writeFetchedTicket(root, jira.Sprint{ID: 3, Name: "Sprint A"}, jira.IssueTicket{ID: "PROJ-3", Title: "Unsafe"})
	if err == nil {
		t.Fatal("expected symlink sprint dir to be rejected")
	}
}

func TestWriteFetchedTicketRejectsSymlinkTicketFile(t *testing.T) {
	root := t.TempDir()
	sprintDir := filepath.Join(root, "Sprint A")
	if err := os.MkdirAll(sprintDir, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside.md")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(sprintDir, "PROJ-4.md")); err != nil {
		t.Skipf("symlink not supported on this system: %v", err)
	}
	err := writeFetchedTicket(root, jira.Sprint{ID: 4, Name: "Sprint A"}, jira.IssueTicket{ID: "PROJ-4", Title: "Unsafe file"})
	if err == nil {
		t.Fatal("expected symlink ticket file to be rejected")
	}
}
