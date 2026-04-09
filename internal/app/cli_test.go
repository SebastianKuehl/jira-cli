package app

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/sebastian/jira-cli/internal/config"
	"github.com/sebastian/jira-cli/internal/jira"
)

func TestAppRunWithoutArgsPrintsHelp(t *testing.T) {
	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	os.Stdout = w
	defer func() { os.Stdout = origStdout }()

	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	if err := New().Run([]string{}); err != nil {
		t.Fatal(err)
	}

	_ = w.Close()
	output := <-done
	if !strings.Contains(output, "Usage: jira <command> [flags]") {
		t.Fatalf("expected help output, got %q", output)
	}
	if strings.Contains(output, "[<target>]") || strings.Contains(output, " [flags]\n    Remove config, sprint folder, or ticket file.") {
		t.Fatalf("expected normalized command summaries, got %q", output)
	}
	if !strings.Contains(output, "  rm <target>") {
		t.Fatalf("expected rm summary without optional brackets, got %q", output)
	}
}

func TestAppRunVersionFlagPrintsVersion(t *testing.T) {
	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	os.Stdout = w
	defer func() { os.Stdout = origStdout }()

	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	originalVersion := Version
	Version = "1.2.3"
	defer func() { Version = originalVersion }()

	if err := New().Run([]string{"--version"}); err != nil {
		t.Fatal(err)
	}

	_ = w.Close()
	output := strings.TrimSpace(<-done)
	if output != "1.2.3" {
		t.Fatalf("expected version output, got %q", output)
	}
}

func TestSubcommandHelpOmitsBracketsAndFlags(t *testing.T) {
	output := renderHelp(t, []string{"rm"})
	if strings.Contains(output, "[<target>]") {
		t.Fatalf("expected target placeholder without brackets, got %q", output)
	}
	if strings.Contains(output, "Usage: jira rm <target> [flags]") {
		t.Fatalf("expected rm usage without [flags], got %q", output)
	}
	if !strings.Contains(output, "Usage: jira rm <target>") {
		t.Fatalf("expected normalized rm usage, got %q", output)
	}
}

func renderHelp(t *testing.T, args []string) string {
	t.Helper()

	parser, err := newParser(&CLI{})
	if err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	parser.Stdout = &buf

	ctx, err := kong.Trace(parser, args)
	if err != nil {
		t.Fatal(err)
	}
	if err := ctx.PrintUsage(false); err != nil {
		t.Fatal(err)
	}

	return buf.String()
}

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

func TestSelectedSprintsRejectsAmbiguousMatch(t *testing.T) {
	sprints := []jira.Sprint{
		{ID: 11, Name: "E51(S4).DevS201"},
		{ID: 12, Name: "E52(S4).QaS201"},
	}

	got, err := selectedSprints(sprints, "201")
	if err == nil {
		t.Fatalf("expected ambiguity error, got %#v", got)
	}
	if !strings.Contains(err.Error(), "matches: E51(S4).DevS201, E52(S4).QaS201") {
		t.Fatalf("expected detailed matches, got %v", err)
	}
}

func TestSelectedSprintsUsesExactMatchBeforeApproximation(t *testing.T) {
	sprints := []jira.Sprint{
		{ID: 11, Name: "Sprint 12"},
		{ID: 12, Name: "Other Sprint"},
	}

	got, err := selectedSprints(sprints, "12")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != 12 {
		t.Fatalf("expected exact sprint id match, got %#v", got)
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

func TestSprintOutputDirRejectsPathTraversal(t *testing.T) {
	root := t.TempDir()
	dir, err := sprintOutputDir(root, jira.Sprint{Name: "../outside"})
	if err != nil {
		t.Fatalf("expected sanitized sprint folder to be accepted, got %v", err)
	}
	if filepath.Base(dir) != "..-outside" {
		t.Fatalf("expected sanitized sprint dir, got %q", dir)
	}
}

func TestWriteFetchedTicketCreatesMissingBasePath(t *testing.T) {
	root := filepath.Join(t.TempDir(), "missing")
	if err := writeFetchedTicket(root, jira.Sprint{Name: "Sprint-1"}, jira.IssueTicket{ID: "VW-1", Title: "Title"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "Sprint-1", "VW-1.md")); err != nil {
		t.Fatalf("expected ticket file written under created base path, stat err=%v", err)
	}
}

func TestSprintFolderNameSanitizesPathSeparators(t *testing.T) {
	if sprintFolderName(jira.Sprint{Name: "A/B"}) != "A-B" {
		t.Fatalf("expected path separators sanitized, got %q", sprintFolderName(jira.Sprint{Name: "A/B"}))
	}
}

func TestFetchCmdRejectsSprintAndTicketTogether(t *testing.T) {
	cmd := &FetchCmd{Sprint: "Sprint-1", Ticket: "VW-2"}
	err := cmd.Run(&Context{CLI: &CLI{Cfg: config.Config{BasePath: t.TempDir()}}})
	if err == nil {
		t.Fatal("expected conflicting fetch args to fail")
	}
}
