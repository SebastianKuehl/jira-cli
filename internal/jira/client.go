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
	type projectSearchResponse struct {
		Values []Project `json:"values"`
		IsLast bool      `json:"isLast"`
		Total  int       `json:"total"`
	}

	startAt := 0
	maxResults := 100
	out := make([]Project, 0)

	for {
		u, err := url.Parse(c.BaseURL + "/rest/api/3/project/search")
		if err != nil {
			return nil, err
		}
		q := u.Query()
		q.Set("maxResults", strconv.Itoa(maxResults))
		q.Set("startAt", strconv.Itoa(startAt))
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
			return nil, fmt.Errorf("jira project list failed with status %s", resp.Status)
		}

		var searchResp projectSearchResponse
		if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
			resp.Body.Close()
			return nil, err
		}
		resp.Body.Close()

		out = append(out, searchResp.Values...)
		if searchResp.IsLast || len(searchResp.Values) == 0 || len(out) >= searchResp.Total {
			break
		}
		startAt += len(searchResp.Values)
	}
	return out, nil
}

func (c *Client) TestConnection(ctx context.Context) error {
	if c.BaseURL == "" || c.Token == "" {
		return errors.New("missing jira credentials")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/rest/api/3/myself", nil)
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
