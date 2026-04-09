package app

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	"github.com/sebastian/jira-cli/internal/config"
)

func TestRmSprintCmdRun(t *testing.T) {
	root := t.TempDir()
	sprintPath := filepath.Join(root, "Sprint-1")
	if err := os.MkdirAll(sprintPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sprintPath, "VW-1.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := &Context{CLI: &CLI{Cfg: config.Config{BasePath: root}}}
	cmd := &RmSprintCmd{Sprint: "Sprint-1"}
	if err := cmd.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sprintPath); !os.IsNotExist(err) {
		t.Fatalf("expected sprint folder removed, stat err=%v", err)
	}
}

func TestRmSprintCmdRejectsDot(t *testing.T) {
	root := t.TempDir()
	ctx := &Context{CLI: &CLI{Cfg: config.Config{BasePath: root}}}
	cmd := &RmSprintCmd{Sprint: "."}
	if err := cmd.Run(ctx); err == nil {
		t.Fatal("expected dot sprint to be rejected")
	}
}

func TestRmSprintCmdRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(outside, "ExternalSprint"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink not supported on this system: %v", err)
	}

	ctx := &Context{CLI: &CLI{Cfg: config.Config{BasePath: root}}}
	cmd := &RmSprintCmd{Sprint: "link/ExternalSprint"}
	if err := cmd.Run(ctx); err == nil {
		t.Fatal("expected symlink escape to be rejected")
	}
}

func TestRmTicketCmdRun(t *testing.T) {
	root := t.TempDir()
	sprintPath := filepath.Join(root, "Sprint-1")
	if err := os.MkdirAll(sprintPath, 0o755); err != nil {
		t.Fatal(err)
	}
	ticketPath := filepath.Join(sprintPath, "VW-2.md")
	if err := os.WriteFile(ticketPath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := &Context{CLI: &CLI{Cfg: config.Config{BasePath: root}}}
	cmd := &RmTicketCmd{ID: "VW-2"}
	if err := cmd.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ticketPath); !os.IsNotExist(err) {
		t.Fatalf("expected ticket removed, stat err=%v", err)
	}
}

func TestRmTicketCmdRejectsInvalidID(t *testing.T) {
	root := t.TempDir()
	ctx := &Context{CLI: &CLI{Cfg: config.Config{BasePath: root}}}
	cmd := &RmTicketCmd{ID: ".env"}
	if err := cmd.Run(ctx); err == nil {
		t.Fatal("expected invalid ticket id to fail")
	}
}

func TestRmCmdRunRemovesSprintWithoutSubcommand(t *testing.T) {
	root := t.TempDir()
	sprintPath := filepath.Join(root, "Sprint Alpha")
	if err := os.MkdirAll(sprintPath, 0o755); err != nil {
		t.Fatal(err)
	}

	ctx := &Context{CLI: &CLI{Cfg: config.Config{BasePath: root}}}
	cmd := &RmCmd{Target: "Sprint Alpha"}
	if err := cmd.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sprintPath); !os.IsNotExist(err) {
		t.Fatalf("expected sprint removed, stat err=%v", err)
	}
}

func TestRmCmdRunRemovesTicketWithoutSubcommand(t *testing.T) {
	root := t.TempDir()
	sprintPath := filepath.Join(root, "Sprint-1")
	if err := os.MkdirAll(sprintPath, 0o755); err != nil {
		t.Fatal(err)
	}
	ticketPath := filepath.Join(sprintPath, "VW-2.md")
	if err := os.WriteFile(ticketPath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := &Context{CLI: &CLI{Cfg: config.Config{BasePath: root}}}
	cmd := &RmCmd{Target: "vw-2"}
	if err := cmd.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ticketPath); !os.IsNotExist(err) {
		t.Fatalf("expected ticket removed, stat err=%v", err)
	}
}

func TestRmCommandParsesDirectTarget(t *testing.T) {
	parser, err := kong.New(&CLI{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.Parse([]string{"rm", "VW-2"}); err != nil {
		t.Fatalf("expected direct rm target to parse, got %v", err)
	}
}

func TestRmCommandStillParsesConfigSubcommand(t *testing.T) {
	parser, err := kong.New(&CLI{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.Parse([]string{"rm", "config"}); err != nil {
		t.Fatalf("expected rm config to parse, got %v", err)
	}
}

func TestResolveRemovalTargetPromptsForAmbiguousTarget(t *testing.T) {
	var out bytes.Buffer
	got, err := resolveRemovalTarget("VW-2", []string{"sprint", "ticket"}, bufio.NewReader(strings.NewReader("2\n")), &out, true)
	if err != nil {
		t.Fatal(err)
	}
	if got != "ticket" {
		t.Fatalf("expected ticket selection, got %q", got)
	}
	if !strings.Contains(out.String(), `Target "VW-2" is ambiguous:`) {
		t.Fatalf("expected ambiguity prompt, got %q", out.String())
	}
}

func TestResolveRemovalTargetErrorsWhenNonInteractive(t *testing.T) {
	got, err := resolveRemovalTarget("VW-2", []string{"sprint", "ticket"}, bufio.NewReader(strings.NewReader("")), &bytes.Buffer{}, false)
	if err == nil {
		t.Fatalf("expected ambiguity error, got %q", got)
	}
}

func TestRmCommandParsesExplicitTicketFlag(t *testing.T) {
	parser, err := kong.New(&CLI{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.Parse([]string{"rm", "--ticket", "VW-2"}); err != nil {
		t.Fatalf("expected explicit ticket flag to parse, got %v", err)
	}
}

func TestRmCmdRejectsConfigFlagWithExtraTarget(t *testing.T) {
	cmd := &RmCmd{Config: true, Target: "VW-2"}
	err := cmd.Run(&Context{CLI: &CLI{}})
	if err == nil {
		t.Fatal("expected conflicting config target to fail")
	}
}
