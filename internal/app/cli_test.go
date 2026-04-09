package app

import (
	"testing"

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
