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

package webtests

import (
	"encoding/json"
	"net/http"
	"testing"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTaskClaimV2 covers POST /tasks/{projecttask}/claim. It drives the
// Echo+Huma stack directly because webHandlerTestV2's buildURL only models
// base[/{id}] paths, not action sub-paths.
//
// Fixture topology: task 19 sits in bucket 10 of kanban view 40 on project 10
// (other bucket: 26), unassigned. Task 18 is in project 9, where testuser1 is
// read-only. Task 34 is in project 20, where testuser1 has no access at all.
func TestTaskClaimV2(t *testing.T) {
	const claimBody = `{"project_view_id":40,"bucket_id":26,"expected_bucket_id":10}`

	bucketOf := func(t *testing.T, taskID int64) int64 {
		s := db.NewSession()
		defer s.Close()
		tb := &models.TaskBucket{}
		_, err := s.Where("task_id = ? AND project_view_id = ?", taskID, 40).Get(tb)
		require.NoError(t, err)
		return tb.BucketID
	}

	assigneeIDs := func(t *testing.T, body []byte) []int64 {
		var resp struct {
			Assignees []struct {
				ID int64 `json:"id"`
			} `json:"assignees"`
		}
		require.NoError(t, json.Unmarshal(body, &resp))
		ids := make([]int64, 0, len(resp.Assignees))
		for _, a := range resp.Assignees {
			ids = append(ids, a.ID)
		}
		return ids
	}

	t.Run("claims an unassigned task, moving it and assigning the caller", func(t *testing.T) {
		e, err := setupTestEnv()
		require.NoError(t, err)
		token := humaTokenFor(t, &testuser1)

		rec := humaRequest(t, e, http.MethodPost, "/api/v2/tasks/19/claim", claimBody, token, "")
		require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
		assert.Equal(t, []int64{1}, assigneeIDs(t, rec.Body.Bytes()))
		assert.Equal(t, int64(26), bucketOf(t, 19))
	})

	t.Run("re-claiming a task you hold is idempotent", func(t *testing.T) {
		e, err := setupTestEnv()
		require.NoError(t, err)
		token := humaTokenFor(t, &testuser1)

		rec := humaRequest(t, e, http.MethodPost, "/api/v2/tasks/19/claim", claimBody, token, "")
		require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
		// Same body again: expected_bucket_id is now stale, but a task the
		// caller already holds must not be refused on the guard.
		rec = humaRequest(t, e, http.MethodPost, "/api/v2/tasks/19/claim", claimBody, token, "")
		require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
		assert.Equal(t, []int64{1}, assigneeIDs(t, rec.Body.Bytes()))
	})

	t.Run("a second writer racing for the same task gets 409", func(t *testing.T) {
		e, err := setupTestEnv()
		require.NoError(t, err)

		rec := humaRequest(t, e, http.MethodPost, "/api/v2/tasks/19/claim", claimBody, humaTokenFor(t, &testuser1), "")
		require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())

		rec = humaRequest(t, e, http.MethodPost, "/api/v2/tasks/19/claim", `{"project_view_id":40,"bucket_id":26}`, humaTokenFor(t, &testuser6), "")
		require.Equal(t, http.StatusConflict, rec.Code, "body: %s", rec.Body.String())
		assert.Contains(t, rec.Body.String(), `"code":4034`)
		// The loser must not have been added as a second assignee.
		assert.Equal(t, int64(26), bucketOf(t, 19))
	})

	t.Run("a task that left the expected bucket gets 409", func(t *testing.T) {
		e, err := setupTestEnv()
		require.NoError(t, err)
		token := humaTokenFor(t, &testuser1)

		rec := humaRequest(t, e, http.MethodPost, "/api/v2/tasks/19/claim", `{"project_view_id":40,"bucket_id":26,"expected_bucket_id":26}`, token, "")
		require.Equal(t, http.StatusConflict, rec.Code, "body: %s", rec.Body.String())
		assert.Contains(t, rec.Body.String(), `"code":4035`)
		assert.Equal(t, int64(10), bucketOf(t, 19), "a refused claim must not move the task")
	})

	t.Run("read-only access is forbidden", func(t *testing.T) {
		e, err := setupTestEnv()
		require.NoError(t, err)
		token := humaTokenFor(t, &testuser1)

		rec := humaRequest(t, e, http.MethodPost, "/api/v2/tasks/18/claim", `{"project_view_id":36,"bucket_id":25}`, token, "")
		require.Equal(t, http.StatusForbidden, rec.Code, "body: %s", rec.Body.String())
	})

	t.Run("no access is forbidden", func(t *testing.T) {
		e, err := setupTestEnv()
		require.NoError(t, err)
		token := humaTokenFor(t, &testuser1)

		rec := humaRequest(t, e, http.MethodPost, "/api/v2/tasks/34/claim", `{"project_view_id":80,"bucket_id":5}`, token, "")
		require.Equal(t, http.StatusForbidden, rec.Code, "body: %s", rec.Body.String())
	})

	t.Run("a bucket from another view is rejected", func(t *testing.T) {
		e, err := setupTestEnv()
		require.NoError(t, err)
		token := humaTokenFor(t, &testuser1)

		// Bucket 1 belongs to view 4 on project 1, not to view 40.
		rec := humaRequest(t, e, http.MethodPost, "/api/v2/tasks/19/claim", `{"project_view_id":40,"bucket_id":1}`, token, "")
		assert.NotEqual(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
		assert.Equal(t, int64(10), bucketOf(t, 19))
	})

	t.Run("nonexistent task", func(t *testing.T) {
		e, err := setupTestEnv()
		require.NoError(t, err)
		token := humaTokenFor(t, &testuser1)

		rec := humaRequest(t, e, http.MethodPost, "/api/v2/tasks/99999/claim", claimBody, token, "")
		require.Equal(t, http.StatusNotFound, rec.Code, "body: %s", rec.Body.String())
	})

	t.Run("missing bucket in body is a validation error", func(t *testing.T) {
		e, err := setupTestEnv()
		require.NoError(t, err)
		token := humaTokenFor(t, &testuser1)

		rec := humaRequest(t, e, http.MethodPost, "/api/v2/tasks/19/claim", `{"project_view_id":40}`, token, "")
		require.Equal(t, http.StatusUnprocessableEntity, rec.Code, "body: %s", rec.Body.String())
	})
}
