package app

import (
	"os"
	"path/filepath"
	"testing"

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
