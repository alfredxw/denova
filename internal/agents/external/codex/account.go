package codex

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"denova/internal/agents/external"
)

func (c *Client) Status() external.ConnectionState {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return external.ConnectionState{Status: "unavailable", ReasonKey: "agentRuntime.connectionLost"}
	}
	if c.accountStatus == "" {
		return external.ConnectionState{Status: "unchecked"}
	}
	return external.ConnectionState{Status: c.accountStatus}
}

func (c *Client) Check(ctx context.Context) (external.ConnectionState, error) {
	ctx, cancel := context.WithTimeout(ctx, infrastructureTimeout)
	defer cancel()
	var response struct {
		Account *struct {
			Type string `json:"type"`
		} `json:"account"`
		RequiresAuth bool `json:"requiresOpenaiAuth"`
	}
	if err := c.call(ctx, "account/read", map[string]bool{"refreshToken": false}, &response); err != nil {
		c.mu.Lock()
		c.accountStatus = "unavailable"
		c.mu.Unlock()
		return external.ConnectionState{}, err
	}
	status := "ready"
	if response.RequiresAuth && response.Account == nil {
		status = "auth_required"
	}
	if response.Account != nil && response.Account.Type != "apiKey" && response.Account.Type != "chatgpt" {
		status = "incompatible"
	}
	c.mu.Lock()
	c.accountStatus = status
	c.mu.Unlock()
	return c.Status(), nil
}

func (c *Client) Models(ctx context.Context) (external.Models, error) {
	ctx, cancel := context.WithTimeout(ctx, infrastructureTimeout)
	defer cancel()
	result := external.Models{Items: []external.Model{}}
	cursor := ""
	seen := map[string]bool{}
	for {
		var page struct {
			Data []struct {
				ID          string `json:"id"`
				Model       string `json:"model"`
				DisplayName string `json:"displayName"`
				Default     bool   `json:"isDefault"`
				Hidden      bool   `json:"hidden"`
				Efforts     []struct {
					Effort string `json:"reasoningEffort"`
				} `json:"supportedReasoningEfforts"`
				DefaultEffort string `json:"defaultReasoningEffort"`
			} `json:"data"`
			Next string `json:"nextCursor"`
		}
		params := map[string]any{"limit": 100, "includeHidden": false}
		if cursor != "" {
			params["cursor"] = cursor
		}
		if err := c.call(ctx, "model/list", params, &page); err != nil {
			return external.Models{}, err
		}
		for _, item := range page.Data {
			if item.Hidden {
				continue
			}
			// model is the turn/start slug; id can be a distinct picker identity.
			if strings.TrimSpace(item.Model) == "" {
				return external.Models{}, errors.New("App Server model has no executable ID")
			}
			model := external.Model{ID: item.Model, DisplayName: item.DisplayName, DefaultEffort: item.DefaultEffort, Efforts: []string{}}
			for _, effort := range item.Efforts {
				model.Efforts = append(model.Efforts, effort.Effort)
			}
			result.Items = append(result.Items, model)
			if item.Default {
				result.DefaultID = model.ID
			}
		}
		if page.Next == "" {
			return result, nil
		}
		if seen[page.Next] || len(result.Items) > 10000 {
			return external.Models{}, errors.New("App Server model catalog pagination is invalid")
		}
		seen[page.Next], cursor = true, page.Next
	}
}

func (c *Client) accountNotification(msg packet) bool {
	switch msg.Method {
	case "account/updated":
		var update struct {
			Mode *string `json:"authMode"`
		}
		if json.Unmarshal(msg.Params, &update) != nil {
			c.fail(errors.New("invalid account update"))
			return true
		}
		c.mu.Lock()
		c.accountStatus = "auth_required"
		if update.Mode != nil {
			switch *update.Mode {
			case "apikey", "chatgpt":
				c.accountStatus = "ready"
			default:
				c.accountStatus = "incompatible"
			}
		}
		c.mu.Unlock()
		return true
	}
	return false
}
