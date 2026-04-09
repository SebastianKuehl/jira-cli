package jira

import (
	"bytes"
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
}

type IssueTicket struct {
	ID       string
	Title    string
	Assignee string
	Reporter string
	State    string
	PRLink   string
}

type User struct {
	Name        string `json:"name"`
	AccountID   string `json:"accountId"`
	DisplayName string `json:"displayName"`
	EmailAddr   string `json:"emailAddress"`
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

func (c *Client) GetCurrentUser(ctx context.Context) (User, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/rest/api/2/myself", nil)
	if err != nil {
		return User{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return User{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return User{}, fmt.Errorf("get current user failed with status %s", resp.Status)
	}
	var u User
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return User{}, err
	}
	return u, nil
}

// SearchAssignableUsers fetches users that can be assigned to issueKey, optionally
// filtered by query. Uses the internal Jira endpoint which returns a {"users":[...]}
// envelope and supports server-side filtering via the query parameter.
func (c *Client) SearchAssignableUsers(ctx context.Context, issueKey, query string) ([]User, error) {
	u, err := url.Parse(c.BaseURL + "/rest/internal/2/users/assignee")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("issueKey", issueKey)
	q.Set("maxResults", "100")
	q.Set("query", query)
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
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("list assignable users failed with status %s", resp.Status)
	}
	var envelope struct {
		Users []User `json:"users"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, err
	}
	return envelope.Users, nil
}

func (c *Client) SearchUsers(ctx context.Context, query string) ([]User, error) {
	u, err := url.Parse(c.BaseURL + "/rest/api/2/user/search")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("query", query)
	q.Set("maxResults", "20")
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
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("user search failed with status %s", resp.Status)
	}
	var users []User
	if err := json.NewDecoder(resp.Body).Decode(&users); err != nil {
		return nil, err
	}
	return users, nil
}

// AssignTicket assigns a Jira issue to the given user. Pass an empty user to unassign.
func (c *Client) AssignTicket(ctx context.Context, issueKey string, user *User) error {
	var body []byte
	var err error
	if user == nil {
		body, err = json.Marshal(map[string]interface{}{"name": nil})
	} else if user.AccountID != "" {
		body, err = json.Marshal(map[string]string{"accountId": user.AccountID})
	} else {
		body, err = json.Marshal(map[string]string{"name": user.Name})
	}
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut,
		fmt.Sprintf("%s/rest/api/2/issue/%s/assignee", c.BaseURL, issueKey),
		bytes.NewReader(body),
	)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("assign ticket failed with status %s", resp.Status)
	}
	return nil
}
