package app

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sebastian/jira-cli/internal/config"
	"github.com/sebastian/jira-cli/internal/jira"
)

func TestFindSprintMatchesExactName(t *testing.T) {
	sprints := []jira.Sprint{
		{ID: 11, Name: "E51(S4).DevS201"},
	}

	got, err := findSprint(sprints, "e51(s4).devs201")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ID != 11 {
		t.Fatalf("expected exact name match, got %#v", got)
	}
}

func TestFindSprintMatchesExactSprintID(t *testing.T) {
	sprints := []jira.Sprint{
		{ID: 201, Name: "Sprint Name"},
	}

	got, err := findSprint(sprints, "201")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Name != "Sprint Name" {
		t.Fatalf("expected sprint id match, got %#v", got)
	}
}

func TestFindSprintMatchesEmbeddedNumericToken(t *testing.T) {
	sprints := []jira.Sprint{
		{ID: 11, Name: "E51(S4).DevS201"},
		{ID: 12, Name: "Other Sprint"},
	}

	got, err := findSprint(sprints, "201")
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.Name != "E51(S4).DevS201" {
		t.Fatalf("expected embedded numeric token match, got %#v", got)
	}
}

func TestFindSprintRejectsPartialNumericMatch(t *testing.T) {
	sprints := []jira.Sprint{
		{ID: 11, Name: "E51(S4).DevS201"},
	}

	got, err := findSprint(sprints, "20")
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("expected no partial numeric match, got %#v", got)
	}
}

func TestFindSprintRejectsAmbiguousNumericMatch(t *testing.T) {
	sprints := []jira.Sprint{
		{ID: 11, Name: "E51(S4).DevS201"},
		{ID: 12, Name: "E52(S4).QaS201"},
	}

	got, err := findSprint(sprints, "201")
	if err == nil {
		t.Fatalf("expected ambiguity error, got sprint %#v", got)
	}
}

func TestResolveFetchSprintSelectionPromptsForAmbiguousMatch(t *testing.T) {
	sprints := []jira.Sprint{
		{ID: 11, Name: "E51(S4).DevS201"},
		{ID: 12, Name: "E52(S4).QaS201"},
	}

	var out bytes.Buffer
	got, err := resolveFetchSprintSelection(sprints, "201", bufio.NewReader(strings.NewReader("2\n")), &out, true)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ID != 12 {
		t.Fatalf("expected second sprint selection, got %#v", got)
	}
	if !strings.Contains(out.String(), `Sprint "201" is ambiguous. Select a sprint:`) {
		t.Fatalf("expected ambiguity prompt, got %q", out.String())
	}
}

func TestResolveFetchSprintSelectionReturnsDetailedErrorWhenNonInteractive(t *testing.T) {
	sprints := []jira.Sprint{
		{ID: 11, Name: "E51(S4).DevS201"},
		{ID: 12, Name: "E52(S4).QaS201"},
	}

	got, err := resolveFetchSprintSelection(sprints, "201", bufio.NewReader(strings.NewReader("")), &bytes.Buffer{}, false)
	if err == nil {
		t.Fatalf("expected ambiguity error, got sprint %#v", got)
	}
	if !strings.Contains(err.Error(), "matches: E51(S4).DevS201, E52(S4).QaS201") {
		t.Fatalf("expected detailed matches, got %v", err)
	}
}

func TestIsTicketIDAcceptsLowercaseInput(t *testing.T) {
	if !isTicketID("vw-123") {
		t.Fatal("expected lowercase ticket id to be accepted")
	}
}

func TestRmTicketAcceptsLowercaseInput(t *testing.T) {
	root := t.TempDir()
	sprintPath := filepath.Join(root, "Sprint-1")
	if err := os.MkdirAll(sprintPath, 0o755); err != nil {
		t.Fatal(err)
	}
	ticketPath := filepath.Join(sprintPath, "VW-2.md")
	if err := os.WriteFile(ticketPath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := RmTicketCmd{ID: "vw-2"}
	ctx := &Context{CLI: &CLI{Cfg: config.Config{BasePath: root}}}
	if err := cmd.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ticketPath); !os.IsNotExist(err) {
		t.Fatalf("expected ticket removed, stat err=%v", err)
	}
}

func TestWriteTicketMarkdownRejectsPathTraversal(t *testing.T) {
	root := t.TempDir()
	if err := writeTicketMarkdown(root, "../outside", jira.IssueTicket{ID: "VW-1", Title: "Title"}); err != nil {
		t.Fatalf("expected sanitized sprint folder to be accepted, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "..%2Foutside", "VW-1.md")); err != nil {
		t.Fatalf("expected sanitized sprint file to be written, stat err=%v", err)
	}
}

func TestWriteTicketMarkdownRejectsSymlinkSprintDir(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(root, "Sprint-1")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink not supported on this system: %v", err)
	}
	err := writeTicketMarkdown(root, "Sprint-1", jira.IssueTicket{ID: "VW-1", Title: "Title"})
	if err == nil {
		t.Fatal("expected symlink sprint directory to be rejected")
	}
}

func TestWriteTicketMarkdownCreatesMissingBasePath(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")
	if err := writeTicketMarkdown(root, "Sprint-1", jira.IssueTicket{ID: "VW-1", Title: "Title"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "Sprint-1", "VW-1.md")); err != nil {
		t.Fatalf("expected ticket file written under created base path, stat err=%v", err)
	}
}

func TestSanitizeSprintFolderNamePreservesDistinctNames(t *testing.T) {
	if sanitizeSprintFolderName("A/B") == sanitizeSprintFolderName("A-B") {
		t.Fatal("expected distinct sprint folder names")
	}
}

func TestFetchCmdRejectsSprintAndTicketTogether(t *testing.T) {
	cmd := &FetchCmd{Sprint: "Sprint-1", Ticket: "VW-2"}
	err := cmd.Run(&Context{CLI: &CLI{Cfg: config.Config{BasePath: t.TempDir()}}})
	if err == nil {
		t.Fatal("expected conflicting fetch args to fail")
	}
}
