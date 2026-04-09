package tickets

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Ticket struct {
	ID       string
	Title    string
	Assignee string
	Reporter string
	State    string
	PRLink   string
}

func ListSprints(basePath string) ([]string, error) {
	entries, err := os.ReadDir(basePath)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out, nil
}

func ListTickets(basePath, sprint string) ([]Ticket, error) {
	if sprint == "" {
		return nil, errors.New("sprint is required")
	}

	baseAbs, err := filepath.Abs(basePath)
	if err != nil {
		return nil, err
	}
	baseResolved, err := filepath.EvalSymlinks(baseAbs)
	if err != nil {
		return nil, err
	}
	sprintPath := filepath.Join(baseAbs, sprint)
	sprintAbs, err := filepath.Abs(sprintPath)
	if err != nil {
		return nil, err
	}
	sprintResolved, err := filepath.EvalSymlinks(sprintAbs)
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(baseResolved, sprintResolved)
	if err != nil {
		return nil, err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("invalid sprint %q", sprint)
	}

	entries, err := os.ReadDir(sprintPath)
	if err != nil {
		return nil, err
	}

	out := make([]Ticket, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		filePath := filepath.Join(sprintPath, name)
		info, err := os.Lstat(filePath)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("ticket file %q must not be a symlink", name)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		ticket, err := parseTicketFile(filePath)
		if err != nil {
			return nil, err
		}
		if ticket.ID == "" {
			ticket.ID = ticketIDFromFilename(name)
		}
		if ticket.Title == "" {
			ticket.Title = ticket.ID
		}
		out = append(out, ticket)
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func parseTicketFile(path string) (Ticket, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Ticket{}, err
	}

	raw := string(b)
	parts := strings.Split(raw, "\n")
	if len(parts) < 3 || strings.TrimSpace(parts[0]) != "---" {
		return Ticket{ID: ticketIDFromFilename(filepath.Base(path))}, nil
	}

	meta := make(map[string]string)
	for i := 1; i < len(parts); i++ {
		line := strings.TrimSpace(parts[i])
		if line == "---" {
			break
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		meta[strings.ToLower(strings.TrimSpace(key))] = strings.Trim(strings.TrimSpace(value), `"'`)
	}

	return Ticket{
		ID:       firstNonEmpty(meta, "id", "key", "ticket", "ticket_id"),
		Title:    firstNonEmpty(meta, "title", "summary"),
		Assignee: firstNonEmpty(meta, "assignee", "assignee_name"),
		Reporter: firstNonEmpty(meta, "reporter", "reporter_name"),
		State:    firstNonEmpty(meta, "workflow_state", "workflow", "status", "state"),
		PRLink:   firstNonEmpty(meta, "pull_request", "pull_request_url", "pr", "pr_url"),
	}, nil
}

func firstNonEmpty(values map[string]string, keys ...string) string {
	for _, key := range keys {
		if v := strings.TrimSpace(values[key]); v != "" {
			return v
		}
	}
	return ""
}

// FindTicketFile searches for a ticket file by ID across all sprint subdirectories
// of basePath. Returns the file path if found, or an error if not found.
func FindTicketFile(basePath, ticketID string) (string, error) {
	sprints, err := ListSprints(basePath)
	if err != nil {
		return "", err
	}
	for _, sprint := range sprints {
		entries, err := os.ReadDir(filepath.Join(basePath, sprint))
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			id := ticketIDFromFilename(name)
			if !strings.EqualFold(id, ticketID) {
				continue
			}
			filePath := filepath.Join(basePath, sprint, name)
			info, err := os.Lstat(filePath)
			if err != nil {
				return "", err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return "", fmt.Errorf("ticket file %q must not be a symlink", name)
			}
			if !info.Mode().IsRegular() {
				continue
			}
			return filePath, nil
		}
	}
	return "", fmt.Errorf("ticket %q not found locally", ticketID)
}

func ticketIDFromFilename(name string) string {
	ext := filepath.Ext(name)
	if ext == "" {
		return name
	}
	return strings.TrimSuffix(name, ext)
}
