package jira

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestGetTicketUsesCachedGETResponse(t *testing.T) {
	t.Setenv("JIRA_CONFIG_DIR", t.TempDir())

	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/2/issue/PROJ-1" {
			http.NotFound(w, r)
			return
		}
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"key":"PROJ-1","fields":{"summary":"Cached title","description":"Body","status":{"name":"Todo"}}}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	first, err := client.GetTicket(context.Background(), "PROJ-1")
	if err != nil {
		t.Fatal(err)
	}
	second, err := client.GetTicket(context.Background(), "PROJ-1")
	if err != nil {
		t.Fatal(err)
	}

	if first.Title != "Cached title" || second.Title != "Cached title" {
		t.Fatalf("expected cached ticket title, got first=%q second=%q", first.Title, second.Title)
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("expected one upstream hit, got %d", got)
	}
}

func TestGetTicketRefreshCacheBypassesStoredResponse(t *testing.T) {
	t.Setenv("JIRA_CONFIG_DIR", t.TempDir())

	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/2/issue/PROJ-1" {
			http.NotFound(w, r)
			return
		}
		count := atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		if count == 1 {
			_, _ = w.Write([]byte(`{"key":"PROJ-1","fields":{"summary":"Stale title","description":"Body","status":{"name":"Todo"}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"key":"PROJ-1","fields":{"summary":"Fresh title","description":"Body","status":{"name":"Todo"}}}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	ticket, err := client.GetTicket(context.Background(), "PROJ-1")
	if err != nil {
		t.Fatal(err)
	}
	if ticket.Title != "Stale title" {
		t.Fatalf("expected initial title, got %q", ticket.Title)
	}

	client.RefreshCache = true
	ticket, err = client.GetTicket(context.Background(), "PROJ-1")
	if err != nil {
		t.Fatal(err)
	}
	if ticket.Title != "Fresh title" {
		t.Fatalf("expected refreshed title, got %q", ticket.Title)
	}

	client.RefreshCache = false
	ticket, err = client.GetTicket(context.Background(), "PROJ-1")
	if err != nil {
		t.Fatal(err)
	}
	if ticket.Title != "Fresh title" {
		t.Fatalf("expected refreshed cache to persist, got %q", ticket.Title)
	}
	if got := atomic.LoadInt32(&hits); got != 2 {
		t.Fatalf("expected two upstream hits, got %d", got)
	}
}

func TestGetTicketCacheScopesByToken(t *testing.T) {
	t.Setenv("JIRA_CONFIG_DIR", t.TempDir())

	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/api/2/issue/PROJ-1" {
			http.NotFound(w, r)
			return
		}
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"key":"PROJ-1","fields":{"summary":"Scoped title","description":"Body","status":{"name":"Todo"}}}`))
	}))
	defer server.Close()

	firstClient := NewClient(server.URL, "token-a")
	if _, err := firstClient.GetTicket(context.Background(), "PROJ-1"); err != nil {
		t.Fatal(err)
	}
	secondClient := NewClient(server.URL, "token-b")
	if _, err := secondClient.GetTicket(context.Background(), "PROJ-1"); err != nil {
		t.Fatal(err)
	}

	if got := atomic.LoadInt32(&hits); got != 2 {
		t.Fatalf("expected separate cache scopes per token, got %d upstream hits", got)
	}
}

func TestTestConnectionBypassesGETCache(t *testing.T) {
	t.Setenv("JIRA_CONFIG_DIR", t.TempDir())

	var ticketHits int32
	var myselfHits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/api/2/issue/PROJ-1":
			atomic.AddInt32(&ticketHits, 1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"key":"PROJ-1","fields":{"summary":"Cached title","description":"Body","status":{"name":"Todo"}}}`))
		case "/rest/api/2/myself":
			atomic.AddInt32(&myselfHits, 1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"user"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	if _, err := client.GetTicket(context.Background(), "PROJ-1"); err != nil {
		t.Fatal(err)
	}
	if err := client.TestConnection(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := client.TestConnection(context.Background()); err != nil {
		t.Fatal(err)
	}

	if got := atomic.LoadInt32(&ticketHits); got != 1 {
		t.Fatalf("expected ticket endpoint cached once, got %d", got)
	}
	if got := atomic.LoadInt32(&myselfHits); got != 2 {
		t.Fatalf("expected connection test to hit network twice, got %d", got)
	}
}

func TestAssignTicketClearsCachedGETs(t *testing.T) {
	t.Setenv("JIRA_CONFIG_DIR", t.TempDir())

	var issueHits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/rest/api/2/issue/PROJ-1":
			count := atomic.AddInt32(&issueHits, 1)
			w.Header().Set("Content-Type", "application/json")
			if count == 1 {
				_, _ = w.Write([]byte(`{"key":"PROJ-1","fields":{"summary":"Before assign","description":"Body","status":{"name":"Todo"}}}`))
				return
			}
			_, _ = w.Write([]byte(`{"key":"PROJ-1","fields":{"summary":"After assign","description":"Body","status":{"name":"In Progress"},"assignee":{"displayName":"Alice"}}}`))
		case r.Method == http.MethodPut && r.URL.Path == "/rest/api/2/issue/PROJ-1/assignee":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	ticket, err := client.GetTicket(context.Background(), "PROJ-1")
	if err != nil {
		t.Fatal(err)
	}
	if ticket.Title != "Before assign" {
		t.Fatalf("expected cached pre-assign ticket, got %q", ticket.Title)
	}
	if err := client.AssignTicket(context.Background(), "PROJ-1", &User{Name: "alice"}); err != nil {
		t.Fatal(err)
	}
	ticket, err = client.GetTicket(context.Background(), "PROJ-1")
	if err != nil {
		t.Fatal(err)
	}
	if ticket.Title != "After assign" {
		t.Fatalf("expected cache invalidation after assign, got %q", ticket.Title)
	}
	if got := atomic.LoadInt32(&issueHits); got != 2 {
		t.Fatalf("expected two GET issue hits after invalidation, got %d", got)
	}
}

func TestSearchTicketsBySprintIDsUsesBulkSearch(t *testing.T) {
	t.Setenv("JIRA_CONFIG_DIR", t.TempDir())

	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/rest/api/2/search" {
			http.NotFound(w, r)
			return
		}
		atomic.AddInt32(&hits, 1)
		body, _ := io.ReadAll(r.Body)
		if got := string(body); !strings.Contains(got, "sprint in (23, 24)") {
			t.Fatalf("expected sprint JQL in request body, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"total":1,"issues":[{"key":"PROJ-7","fields":{"summary":"Bulk fetched","description":"Body","labels":["cli"],"status":{"name":"Todo"}}}]}`))
	}))
	defer server.Close()

	client := NewClient(server.URL, "token")
	tickets, err := client.SearchTicketsBySprintIDs(context.Background(), []int{24, 23})
	if err != nil {
		t.Fatal(err)
	}
	if len(tickets) != 1 || tickets[0].ID != "PROJ-7" || tickets[0].Description != "Body" {
		t.Fatalf("expected bulk-fetched ticket, got %#v", tickets)
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("expected one bulk search request, got %d", got)
	}
}
