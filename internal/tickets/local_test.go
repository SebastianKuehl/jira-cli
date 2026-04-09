package tickets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListSprintsAndTickets(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "Sprint A"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "Sprint B"), 0o755); err != nil {
		t.Fatal(err)
	}

	ticketBody := `---
id: VW-1
title: Implement feature
assignee: Alice
reporter: Bob
workflow_state: In Progress
pull_request_url: https://example.com/pr/1
---
Body`

	if err := os.WriteFile(filepath.Join(root, "Sprint A", "VW-1.md"), []byte(ticketBody), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Sprint A", "VW-2.md"), []byte("no-frontmatter"), 0o644); err != nil {
		t.Fatal(err)
	}

	sprints, err := ListSprints(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(sprints) != 2 || sprints[0] != "Sprint A" || sprints[1] != "Sprint B" {
		t.Fatalf("unexpected sprints: %#v", sprints)
	}

	tickets, err := ListTickets(root, "Sprint A")
	if err != nil {
		t.Fatal(err)
	}
	if len(tickets) != 2 {
		t.Fatalf("expected 2 tickets, got %d", len(tickets))
	}
	if tickets[0].ID != "VW-1" || tickets[0].Title != "Implement feature" {
		t.Fatalf("unexpected parsed ticket: %#v", tickets[0])
	}
	if tickets[0].PRLink != "https://example.com/pr/1" {
		t.Fatalf("unexpected pr link: %#v", tickets[0])
	}
	if tickets[1].ID != "VW-2" || tickets[1].Title != "VW-2" {
		t.Fatalf("expected filename fallback for ticket 2, got %#v", tickets[1])
	}
}

func TestListTicketsRejectsPathTraversal(t *testing.T) {
	root := t.TempDir()
	if _, err := ListTickets(root, "../outside"); err == nil {
		t.Fatal("expected traversal sprint to fail")
	}
}

func TestListTicketsRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "VW-999.md"), []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "Sprint-Link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink not supported on this system: %v", err)
	}
	if _, err := ListTickets(root, "Sprint-Link"); err == nil {
		t.Fatal("expected symlink sprint to be rejected")
	}
}

func TestListTicketsRejectsSymlinkTicketFile(t *testing.T) {
	root := t.TempDir()
	sprint := filepath.Join(root, "Sprint-A")
	if err := os.Mkdir(sprint, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(root, "outside.md")
	if err := os.WriteFile(outside, []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(sprint, "VW-100.md")); err != nil {
		t.Skipf("symlink not supported on this system: %v", err)
	}
	if _, err := ListTickets(root, "Sprint-A"); err == nil {
		t.Fatal("expected symlink ticket file to be rejected")
	}
}
