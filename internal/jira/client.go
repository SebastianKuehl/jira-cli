package jira

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var ErrNotImplemented = errors.New("not implemented")

type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

type Project struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

type Board struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type Sprint struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	State string `json:"state"`
	Goal  string `json:"goal"`
}

type IssueTicket struct {
	ID          string
	Title       string
	Assignee    string
	Reporter    string
	State       string
	PRLink      string
	Description string
	Priority    string
	Labels      []string
	URL         string
}

func NewClient(baseURL, token string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		HTTPClient: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

func (c *Client) ListProjects(ctx context.Context) ([]Project, error) {
	if c.BaseURL == "" || c.Token == "" {
		return nil, errors.New("missing jira credentials")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/rest/api/2/project", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("jira project list failed with status %s", resp.Status)
	}

	var projects []Project
	if err := json.NewDecoder(resp.Body).Decode(&projects); err != nil {
		return nil, err
	}
	return projects, nil
}

func (c *Client) TestConnection(ctx context.Context) error {
	if c.BaseURL == "" || c.Token == "" {
		return errors.New("missing jira credentials")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/rest/api/2/myself", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("jira test failed with status %s", resp.Status)
	}
	return nil
}

func (c *Client) ListBoards(ctx context.Context, projectKey string) ([]Board, error) {
	if c.BaseURL == "" || c.Token == "" {
		return nil, errors.New("missing jira credentials")
	}
	out := make([]Board, 0)
	startAt := 0
	for {
		u, err := url.Parse(c.BaseURL + "/rest/agile/1.0/board")
		if err != nil {
			return nil, err
		}
		q := u.Query()
		q.Set("projectKeyOrId", projectKey)
		q.Set("startAt", strconv.Itoa(startAt))
		q.Set("maxResults", "50")
		u.RawQuery = q.Encode()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.Token)
		req.Header.Set("Accept", "application/json")
		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode >= 300 {
			resp.Body.Close()
			return nil, fmt.Errorf("jira board list failed with status %s", resp.Status)
		}
		var parsed struct {
			StartAt    int     `json:"startAt"`
			MaxResults int     `json:"maxResults"`
			IsLast     bool    `json:"isLast"`
			Values     []Board `json:"values"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
			resp.Body.Close()
			return nil, err
		}
		resp.Body.Close()
		out = append(out, parsed.Values...)
		if parsed.IsLast || len(parsed.Values) == 0 {
			break
		}
		startAt += len(parsed.Values)
	}
	return out, nil
}

func (c *Client) ListSprints(ctx context.Context, boardID int) ([]Sprint, error) {
	if c.BaseURL == "" || c.Token == "" {
		return nil, errors.New("missing jira credentials")
	}
	out := make([]Sprint, 0)
	startAt := 0
	for {
		u, err := url.Parse(fmt.Sprintf("%s/rest/agile/1.0/board/%d/sprint", c.BaseURL, boardID))
		if err != nil {
			return nil, err
		}
		q := u.Query()
		q.Set("state", "active,future,closed")
		q.Set("startAt", strconv.Itoa(startAt))
		q.Set("maxResults", "50")
		u.RawQuery = q.Encode()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.Token)
		req.Header.Set("Accept", "application/json")
		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode >= 300 {
			resp.Body.Close()
			return nil, fmt.Errorf("jira sprint list failed with status %s", resp.Status)
		}
		var parsed struct {
			IsLast bool     `json:"isLast"`
			Values []Sprint `json:"values"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
			resp.Body.Close()
			return nil, err
		}
		resp.Body.Close()
		out = append(out, parsed.Values...)
		if parsed.IsLast || len(parsed.Values) == 0 {
			break
		}
		startAt += len(parsed.Values)
	}
	return out, nil
}

func (c *Client) ListSprintTickets(ctx context.Context, boardID, sprintID int) ([]IssueTicket, error) {
	if c.BaseURL == "" || c.Token == "" {
		return nil, errors.New("missing jira credentials")
	}
	out := make([]IssueTicket, 0)
	startAt := 0
	for {
		u, err := url.Parse(fmt.Sprintf("%s/rest/agile/1.0/board/%d/sprint/%d/issue", c.BaseURL, boardID, sprintID))
		if err != nil {
			return nil, err
		}
		q := u.Query()
		q.Set("startAt", strconv.Itoa(startAt))
		q.Set("maxResults", "50")
		u.RawQuery = q.Encode()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.Token)
		req.Header.Set("Accept", "application/json")
		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode >= 300 {
			resp.Body.Close()
			return nil, fmt.Errorf("jira sprint issue list failed with status %s", resp.Status)
		}
		var parsed struct {
			StartAt    int `json:"startAt"`
			MaxResults int `json:"maxResults"`
			Total      int `json:"total"`
			Issues     []struct {
				Key    string `json:"key"`
				Fields struct {
					Summary string `json:"summary"`
					Status  struct {
						Name string `json:"name"`
					} `json:"status"`
					Assignee *struct {
						DisplayName string `json:"displayName"`
					} `json:"assignee"`
					Reporter *struct {
						DisplayName string `json:"displayName"`
					} `json:"reporter"`
				} `json:"fields"`
			} `json:"issues"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
			resp.Body.Close()
			return nil, err
		}
		resp.Body.Close()
		for _, issue := range parsed.Issues {
			item := IssueTicket{
				ID:    issue.Key,
				Title: issue.Fields.Summary,
				State: issue.Fields.Status.Name,
			}
			if issue.Fields.Assignee != nil {
				item.Assignee = issue.Fields.Assignee.DisplayName
			}
			if issue.Fields.Reporter != nil {
				item.Reporter = issue.Fields.Reporter.DisplayName
			}
			out = append(out, item)
		}
		startAt += len(parsed.Issues)
		if startAt >= parsed.Total || len(parsed.Issues) == 0 {
			break
		}
	}
	return out, nil
}

func (c *Client) GetTicket(ctx context.Context, issueKey string) (IssueTicket, error) {
	if c.BaseURL == "" || c.Token == "" {
		return IssueTicket{}, errors.New("missing jira credentials")
	}
	u := fmt.Sprintf("%s/rest/api/2/issue/%s", c.BaseURL, url.PathEscape(issueKey))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return IssueTicket{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return IssueTicket{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return IssueTicket{}, fmt.Errorf("ticket %q not found", issueKey)
	}
	if resp.StatusCode >= 300 {
		return IssueTicket{}, fmt.Errorf("jira get issue failed with status %s", resp.Status)
	}

	var parsed struct {
		Key    string `json:"key"`
		Fields struct {
			Summary     string          `json:"summary"`
			Description json.RawMessage `json:"description"`
			Priority    *struct {
				Name string `json:"name"`
			} `json:"priority"`
			Labels   []string `json:"labels"`
			Status   struct {
				Name string `json:"name"`
			} `json:"status"`
			Assignee *struct {
				DisplayName string `json:"displayName"`
			} `json:"assignee"`
			Reporter *struct {
				DisplayName string `json:"displayName"`
			} `json:"reporter"`
		} `json:"fields"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return IssueTicket{}, err
	}

	ticket := IssueTicket{
		ID:     parsed.Key,
		Title:  parsed.Fields.Summary,
		State:  parsed.Fields.Status.Name,
		Labels: parsed.Fields.Labels,
		URL:    strings.TrimRight(c.BaseURL, "/") + "/browse/" + parsed.Key,
	}
	ticket.Description = decodeDescription(parsed.Fields.Description)
	if parsed.Fields.Priority != nil {
		ticket.Priority = parsed.Fields.Priority.Name
	}
	if parsed.Fields.Assignee != nil {
		ticket.Assignee = parsed.Fields.Assignee.DisplayName
	}
	if parsed.Fields.Reporter != nil {
		ticket.Reporter = parsed.Fields.Reporter.DisplayName
	}
	return ticket, nil
}

// decodeDescription handles both plain-string (Jira Server/DC wiki markup) and
// Atlassian Document Format object (Jira Cloud) responses for the description field.
func decodeDescription(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	// Plain string (Jira Server / Data Center)
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	// ADF object (Jira Cloud) — extract text nodes recursively
	var node adfNode
	if err := json.Unmarshal(raw, &node); err != nil {
		return ""
	}
	return strings.TrimSpace(extractADFText(&node))
}

type adfNode struct {
	Type    string            `json:"type"`
	Text    string            `json:"text"`
	Content []adfNode         `json:"content"`
	Attrs   map[string]interface{} `json:"attrs"`
}

func extractADFText(n *adfNode) string {
	if n == nil {
		return ""
	}
	if n.Type == "text" {
		return n.Text
	}
	var sb strings.Builder
	for i := range n.Content {
		sb.WriteString(extractADFText(&n.Content[i]))
	}
	// Add newline after block-level nodes so output is readable
	switch n.Type {
	case "paragraph", "heading", "bulletList", "orderedList", "listItem",
		"blockquote", "codeBlock", "rule", "panel":
		sb.WriteString("\n")
	}
	return sb.String()
}
