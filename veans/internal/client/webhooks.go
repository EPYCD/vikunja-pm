// Vikunja is a to-do list application to facilitate your life.
// Copyright 2018-present Vikunja and contributors. All rights reserved.
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package client

import (
	"context"
	"fmt"
)

// Webhook is a project-level outbound webhook target.
type Webhook struct {
	ID        int64    `json:"id,omitempty"`
	ProjectID int64    `json:"project_id,omitempty"`
	TargetURL string   `json:"target_url"`
	Events    []string `json:"events"`
	// Secret is write-only: the server signs deliveries with it and never
	// returns it.
	Secret string `json:"secret,omitempty"`
}

// WebhookEvents lists the event names the server can deliver.
func (c *Client) WebhookEvents(ctx context.Context) ([]string, error) {
	var out []string
	if err := c.Do(ctx, "GET", "/webhooks/events", nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListProjectWebhooks returns the project's webhook targets.
func (c *Client) ListProjectWebhooks(ctx context.Context, projectID int64) ([]*Webhook, error) {
	items, _, err := doList[*Webhook](ctx, c, fmt.Sprintf("/projects/%d/webhooks", projectID), nil)
	if err != nil {
		return nil, err
	}
	return items, nil
}

// CreateProjectWebhook registers a target for the given events.
func (c *Client) CreateProjectWebhook(ctx context.Context, projectID int64, w *Webhook) (*Webhook, error) {
	var out Webhook
	if err := c.Do(ctx, "POST", fmt.Sprintf("/projects/%d/webhooks", projectID), nil, w, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpdateProjectWebhook replaces the events (and, when set, the secret) of a target.
func (c *Client) UpdateProjectWebhook(ctx context.Context, projectID int64, w *Webhook) (*Webhook, error) {
	var out Webhook
	if err := c.Do(ctx, "PUT", fmt.Sprintf("/projects/%d/webhooks/%d", projectID, w.ID), nil, w, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// DeleteProjectWebhook removes a target.
func (c *Client) DeleteProjectWebhook(ctx context.Context, projectID, webhookID int64) error {
	return c.Do(ctx, "DELETE", fmt.Sprintf("/projects/%d/webhooks/%d", projectID, webhookID), nil, nil, nil)
}
