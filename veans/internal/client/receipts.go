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
	"time"
)

// Gate statuses the server accepts on a receipt.
const (
	GatePassed  = "passed"
	GateFailed  = "failed"
	GateSkipped = "skipped"
)

// GateResult is one gate of a CI run.
type GateResult struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	DurationMS int64  `json:"duration_ms"`
}

// TaskReceipt mirrors pkg/models.TaskReceipt: CI's record of a run. Only
// the project's receipt bot may post one; nothing can change or delete it.
type TaskReceipt struct {
	ID                 int64        `json:"id,omitempty"`
	TaskID             int64        `json:"task_id,omitempty"`
	ProjectID          int64        `json:"project_id,omitempty"`
	CommitSHA          string       `json:"commit_sha"`
	Branch             string       `json:"branch,omitempty"`
	Gates              []GateResult `json:"gates"`
	DocsAPIRequired    bool         `json:"docs_api_required"`
	DocsAPIRegenerated bool         `json:"docs_api_regenerated"`
	CIRunURL           string       `json:"ci_run_url,omitempty"`
	Merged             bool         `json:"merged"`
	MergeSHA           string       `json:"merge_sha,omitempty"`
	Passed             bool         `json:"passed,omitempty"`
	PostedBy           *User        `json:"posted_by,omitempty"`
	Created            time.Time    `json:"created,omitempty"`
}

// PostTaskReceipt records a CI run on the task. 403 unless the token belongs
// to the project's receipt bot.
func (c *Client) PostTaskReceipt(ctx context.Context, taskID int64, r *TaskReceipt) (*TaskReceipt, error) {
	var out TaskReceipt
	if err := c.Do(ctx, "POST", fmt.Sprintf("/tasks/%d/receipts", taskID), nil, r, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListTaskReceipts returns the task's receipts, newest first.
func (c *Client) ListTaskReceipts(ctx context.Context, taskID int64) ([]*TaskReceipt, error) {
	items, _, err := doList[*TaskReceipt](ctx, c, fmt.Sprintf("/tasks/%d/receipts", taskID), nil)
	if err != nil {
		return nil, err
	}
	return items, nil
}
