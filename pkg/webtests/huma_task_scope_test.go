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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Fixture topology: task 19 (project 10, write for testuser1) owns
// pkg/models/**. Task 1 (project 1) is leased on pkg/models/tasks.go and task 9
// (project 1, view 4, bucket 1) owns pkg/models/*.go, so 9 conflicts with 1.
// Task 18 is read-only for testuser1; task 34 is inaccessible.
func TestTaskScopeV2(t *testing.T) {
	t.Run("read an existing scope", func(t *testing.T) {
		e, err := setupTestEnv()
		require.NoError(t, err)
		rec := humaRequest(t, e, http.MethodGet, "/api/v2/tasks/19/scope", "", humaTokenFor(t, &testuser1), "")
		require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
		assert.Contains(t, rec.Body.String(), `"paths_owned":["pkg/models/**"]`)
		assert.Contains(t, rec.Body.String(), `"endpoints":["POST /api/v2/tasks/{id}/claim"]`)
	})

	t.Run("a task without a scope reads as empty lists", func(t *testing.T) {
		e, err := setupTestEnv()
		require.NoError(t, err)
		rec := humaRequest(t, e, http.MethodGet, "/api/v2/tasks/10/scope", "", humaTokenFor(t, &testuser1), "")
		require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
		assert.Contains(t, rec.Body.String(), `"paths_owned":[]`)
		assert.Contains(t, rec.Body.String(), `"task_id":10`)
	})

	t.Run("put creates and normalises", func(t *testing.T) {
		e, err := setupTestEnv()
		require.NoError(t, err)
		token := humaTokenFor(t, &testuser1)
		body := `{"paths_owned":["./pkg//models/","frontend/src/**"],"paths_affected":["/docs/"],"endpoints":[" GET /x "],"notes":"<p>n</p>"}`
		rec := humaRequest(t, e, http.MethodPut, "/api/v2/tasks/10/scope", body, token, "")
		require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
		assert.Contains(t, rec.Body.String(), `"paths_owned":["pkg/models","frontend/src/**"]`)
		assert.Contains(t, rec.Body.String(), `"paths_affected":["docs"]`)
		assert.Contains(t, rec.Body.String(), `"endpoints":["GET /x"]`)

		rec = humaRequest(t, e, http.MethodGet, "/api/v2/tasks/10/scope", "", token, "")
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), `"notes":"<p>n</p>"`)
	})

	t.Run("put rejects a path that escapes the repository", func(t *testing.T) {
		e, err := setupTestEnv()
		require.NoError(t, err)
		rec := humaRequest(t, e, http.MethodPut, "/api/v2/tasks/10/scope", `{"paths_owned":["../etc"]}`, humaTokenFor(t, &testuser1), "")
		require.Equal(t, http.StatusBadRequest, rec.Code, "body: %s", rec.Body.String())
		assert.Contains(t, rec.Body.String(), `"code":4038`)
	})

	t.Run("read-only cannot write, no access cannot read", func(t *testing.T) {
		e, err := setupTestEnv()
		require.NoError(t, err)
		token := humaTokenFor(t, &testuser1)
		rec := humaRequest(t, e, http.MethodPut, "/api/v2/tasks/18/scope", `{"paths_owned":["a"]}`, token, "")
		require.Equal(t, http.StatusForbidden, rec.Code, "body: %s", rec.Body.String())
		rec = humaRequest(t, e, http.MethodGet, "/api/v2/tasks/18/scope", "", token, "")
		require.Equal(t, http.StatusOK, rec.Code, "read-only may still read; body: %s", rec.Body.String())
		rec = humaRequest(t, e, http.MethodGet, "/api/v2/tasks/34/scope", "", token, "")
		require.Equal(t, http.StatusForbidden, rec.Code, "body: %s", rec.Body.String())
		rec = humaRequest(t, e, http.MethodGet, "/api/v2/tasks/99999/scope", "", token, "")
		require.Equal(t, http.StatusNotFound, rec.Code, "body: %s", rec.Body.String())
	})

	t.Run("delete removes the scope", func(t *testing.T) {
		e, err := setupTestEnv()
		require.NoError(t, err)
		token := humaTokenFor(t, &testuser1)
		rec := humaRequest(t, e, http.MethodDelete, "/api/v2/tasks/19/scope", "", token, "")
		require.Equal(t, http.StatusNoContent, rec.Code, "body: %s", rec.Body.String())
		rec = humaRequest(t, e, http.MethodGet, "/api/v2/tasks/19/scope", "", token, "")
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), `"paths_owned":[]`)
	})

	t.Run("expand=scope and expand=leases on a task read", func(t *testing.T) {
		e, err := setupTestEnv()
		require.NoError(t, err)
		token := humaTokenFor(t, &testuser1)
		rec := humaRequest(t, e, http.MethodGet, "/api/v2/tasks/19?expand=scope", "", token, "")
		require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
		assert.Contains(t, rec.Body.String(), `"scope":{`)
		assert.Contains(t, rec.Body.String(), `"paths_owned":["pkg/models/**"]`)

		rec = humaRequest(t, e, http.MethodGet, "/api/v2/tasks/1?expand=leases", "", token, "")
		require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
		assert.Contains(t, rec.Body.String(), `"pattern":"pkg/models/tasks.go"`)

		rec = humaRequest(t, e, http.MethodGet, "/api/v2/tasks/10?expand=leases", "", token, "")
		require.Equal(t, http.StatusOK, rec.Code)
		assert.NotContains(t, rec.Body.String(), `"pattern":`, "task 10 holds nothing; like buckets, an empty expand is omitted")
	})
}

func TestTaskPathLeaseV2(t *testing.T) {
	t.Run("list a project's leases with holders", func(t *testing.T) {
		e, err := setupTestEnv()
		require.NoError(t, err)
		rec := humaRequest(t, e, http.MethodGet, "/api/v2/projects/1/leases", "", humaTokenFor(t, &testuser1), "")
		require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
		var resp struct {
			Items []struct {
				Pattern string `json:"pattern"`
				TaskID  int64  `json:"task_id"`
				Task    *struct {
					ID int64 `json:"id"`
				} `json:"task"`
				User *struct {
					ID int64 `json:"id"`
				} `json:"user"`
			} `json:"items"`
			Total int64 `json:"total"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		require.Len(t, resp.Items, 1)
		assert.EqualValues(t, 1, resp.Total)
		assert.Equal(t, "pkg/models/tasks.go", resp.Items[0].Pattern)
		require.NotNil(t, resp.Items[0].Task)
		assert.Equal(t, int64(1), resp.Items[0].Task.ID)
		require.NotNil(t, resp.Items[0].User)
		assert.Equal(t, int64(1), resp.Items[0].User.ID)
	})

	t.Run("no access to the project is forbidden", func(t *testing.T) {
		e, err := setupTestEnv()
		require.NoError(t, err)
		rec := humaRequest(t, e, http.MethodGet, "/api/v2/projects/20/leases", "", humaTokenFor(t, &testuser1), "")
		require.Equal(t, http.StatusForbidden, rec.Code, "body: %s", rec.Body.String())
	})

	t.Run("claim conflicting with a lease is refused with 4037", func(t *testing.T) {
		e, err := setupTestEnv()
		require.NoError(t, err)
		rec := humaRequest(t, e, http.MethodPost, "/api/v2/tasks/9/claim", `{"project_view_id":4,"bucket_id":1,"expected_bucket_id":1}`, humaTokenFor(t, &testuser1), "")
		require.Equal(t, http.StatusConflict, rec.Code, "body: %s", rec.Body.String())
		assert.Contains(t, rec.Body.String(), `"code":4037`)
		assert.Contains(t, rec.Body.String(), `"heldByTaskId":"1"`)
	})

	t.Run("release then claim", func(t *testing.T) {
		e, err := setupTestEnv()
		require.NoError(t, err)
		token := humaTokenFor(t, &testuser1)
		rec := humaRequest(t, e, http.MethodDelete, "/api/v2/tasks/1/leases", "", token, "")
		require.Equal(t, http.StatusNoContent, rec.Code, "body: %s", rec.Body.String())
		rec = humaRequest(t, e, http.MethodGet, "/api/v2/projects/1/leases", "", token, "")
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), `"items":[]`)

		rec = humaRequest(t, e, http.MethodPost, "/api/v2/tasks/9/claim", `{"project_view_id":4,"bucket_id":1,"expected_bucket_id":1}`, token, "")
		require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
		rec = humaRequest(t, e, http.MethodGet, "/api/v2/projects/1/leases", "", token, "")
		assert.Contains(t, rec.Body.String(), `"pattern":"pkg/models/*.go"`)
	})

	t.Run("release requires write access", func(t *testing.T) {
		e, err := setupTestEnv()
		require.NoError(t, err)
		rec := humaRequest(t, e, http.MethodDelete, "/api/v2/tasks/18/leases", "", humaTokenFor(t, &testuser1), "")
		require.Equal(t, http.StatusForbidden, rec.Code, "body: %s", rec.Body.String())
	})
}

func TestTaskReadinessV2(t *testing.T) {
	t.Run("the ready queue of the default bucket", func(t *testing.T) {
		e, err := setupTestEnv()
		require.NoError(t, err)
		rec := humaRequest(t, e, http.MethodGet, "/api/v2/projects/1/views/4/readiness", "", humaTokenFor(t, &testuser1), "")
		require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
		var resp struct {
			Items []struct {
				Task struct {
					ID int64 `json:"id"`
				} `json:"task"`
				Ready          bool     `json:"ready"`
				Reasons        []string `json:"reasons"`
				LeaseConflicts []struct {
					HeldByTaskID int64 `json:"HeldByTaskID"`
				} `json:"lease_conflicts"`
			} `json:"items"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
		byID := map[int64]bool{}
		for _, it := range resp.Items {
			byID[it.Task.ID] = it.Ready
			if it.Task.ID == 9 {
				assert.False(t, it.Ready)
				assert.Equal(t, []string{"lease_conflict"}, it.Reasons)
				require.Len(t, it.LeaseConflicts, 1)
			}
		}
		require.Contains(t, byID, int64(9))
		assert.True(t, byID[10])
	})

	t.Run("explicit bucket and permissions", func(t *testing.T) {
		e, err := setupTestEnv()
		require.NoError(t, err)
		token := humaTokenFor(t, &testuser1)
		rec := humaRequest(t, e, http.MethodGet, "/api/v2/projects/1/views/4/readiness?bucket_id=3", "", token, "")
		require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
		// Fixture bucket 3 holds tasks 2 (done), 6, 7 and 8: done tasks are never queued.
		assert.Contains(t, rec.Body.String(), `"total":3`)
		assert.NotContains(t, rec.Body.String(), `"id":2,"title"`)

		rec = humaRequest(t, e, http.MethodGet, "/api/v2/projects/9/views/36/readiness", "", token, "")
		require.Equal(t, http.StatusOK, rec.Code, "read-only may read the queue; body: %s", rec.Body.String())
		rec = humaRequest(t, e, http.MethodGet, "/api/v2/projects/20/views/80/readiness", "", token, "")
		require.Equal(t, http.StatusForbidden, rec.Code, "body: %s", rec.Body.String())
		rec = humaRequest(t, e, http.MethodGet, "/api/v2/projects/1/views/40/readiness", "", token, "")
		require.Equal(t, http.StatusNotFound, rec.Code, "view 40 is not project 1's; body: %s", rec.Body.String())
	})
}
