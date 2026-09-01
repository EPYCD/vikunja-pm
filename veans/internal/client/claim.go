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

// TaskClaim is the body of POST /tasks/{id}/claim.
type TaskClaim struct {
	ProjectViewID int64 `json:"project_view_id"`
	BucketID      int64 `json:"bucket_id"`
	// ExpectedBucketID, when non-zero, makes the server refuse the claim if
	// the task is no longer in that bucket — the compare-and-swap that stops
	// two agents claiming on stale `list --ready` output.
	ExpectedBucketID int64 `json:"expected_bucket_id,omitempty"`
}

// ClaimTask atomically takes a task: the server checks nobody else holds it,
// moves it into bucketID on viewID and assigns the caller, all in one
// transaction. A CONFLICT error means another user got there first (or the
// task left expectedBucketID). Re-claiming a task the caller already holds
// succeeds without changes.
//
// The returned task comes from the plain read, so its Buckets slice is not
// expanded; call GetTask for the per-view bucket memberships.
func (c *Client) ClaimTask(ctx context.Context, taskID, viewID, bucketID, expectedBucketID int64) (*Task, error) {
	var out Task
	body := &TaskClaim{ProjectViewID: viewID, BucketID: bucketID, ExpectedBucketID: expectedBucketID}
	if err := c.Do(ctx, "POST", fmt.Sprintf("/tasks/%d/claim", taskID), nil, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
