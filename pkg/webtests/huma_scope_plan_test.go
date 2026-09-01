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
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScopeCheckV2(t *testing.T) {
	t.Run("verdicts and permissions", func(t *testing.T) {
		e, err := setupTestEnv()
		require.NoError(t, err)
		token := humaTokenFor(t, &testuser1)

		body := `{"task_ids":[19],"files":["pkg/models/x.go","frontend/src/App.vue"]}`
		rec := humaRequest(t, e, http.MethodPost, "/api/v2/projects/10/scope-check", body, token, "")
		require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
		var res struct {
			OK       bool `json:"ok"`
			Enforced bool `json:"enforced"`
			Strays   int  `json:"strays"`
			Files    []struct {
				Verdict string `json:"verdict"`
			} `json:"files"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &res))
		assert.True(t, res.Enforced)
		assert.False(t, res.OK)
		assert.Equal(t, 1, res.Strays)
		assert.Equal(t, "owned", res.Files[0].Verdict)
		assert.Equal(t, "unscoped", res.Files[1].Verdict)

		rec = humaRequest(t, e, http.MethodPost, "/api/v2/projects/9/scope-check", body, token, "")
		assert.Equal(t, http.StatusOK, rec.Code, "read-only access may check")
		rec = humaRequest(t, e, http.MethodPost, "/api/v2/projects/20/scope-check", body, token, "")
		assert.Equal(t, http.StatusForbidden, rec.Code)
		rec = humaRequest(t, e, http.MethodPost, "/api/v2/projects/10/scope-check", `{"task_ids":[19],"files":[]}`, token, "")
		assert.Equal(t, http.StatusUnprocessableEntity, rec.Code, "files are required")
	})
}

func TestTaskPlanV2(t *testing.T) {
	t.Run("dry run lints, real run creates", func(t *testing.T) {
		e, err := setupTestEnv()
		require.NoError(t, err)
		token := humaTokenFor(t, &testuser1)

		plan := `{"dry_run":true,"tasks":[{"key":"a","title":"A","blocked_by":["b"]},{"key":"b","title":"B","blocked_by":["a"]}]}`
		rec := humaRequest(t, e, http.MethodPost, "/api/v2/projects/1/plan", plan, token, "")
		require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
		assert.Contains(t, rec.Body.String(), `"ok":false`)
		assert.Contains(t, rec.Body.String(), `"dependency_cycle"`)

		plan = `{"tasks":[{"key":"epic","title":"Epic"},{"key":"api","title":"API","parent_key":"epic","scope":{"paths_owned":["pkg/routes/**"],"paths_affected":[],"endpoints":["GET /x"],"notes":""}}]}`
		rec = humaRequest(t, e, http.MethodPost, "/api/v2/projects/1/plan", plan, token, "")
		require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
		var res struct {
			OK      bool `json:"ok"`
			Created bool `json:"created"`
			Tasks   []struct {
				Key        string `json:"key"`
				ID         int64  `json:"id"`
				Identifier string `json:"identifier"`
			} `json:"tasks"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &res))
		assert.True(t, res.OK)
		assert.True(t, res.Created)
		require.Len(t, res.Tasks, 2)
		assert.Equal(t, "epic", res.Tasks[0].Key)
		assert.NotEmpty(t, res.Tasks[0].Identifier)

		rec = humaRequest(t, e, http.MethodGet, "/api/v2/tasks/"+strconv.FormatInt(res.Tasks[1].ID, 10)+"/scope", "", token, "")
		require.Equal(t, http.StatusOK, rec.Code)
		assert.Contains(t, rec.Body.String(), `"paths_owned":["pkg/routes/**"]`)
	})

	t.Run("permissions", func(t *testing.T) {
		e, err := setupTestEnv()
		require.NoError(t, err)
		token := humaTokenFor(t, &testuser1)
		plan := `{"dry_run":true,"tasks":[{"key":"a","title":"A"}]}`
		rec := humaRequest(t, e, http.MethodPost, "/api/v2/projects/9/plan", plan, token, "")
		assert.Equal(t, http.StatusForbidden, rec.Code, "read-only cannot plan; body: %s", rec.Body.String())
		rec = humaRequest(t, e, http.MethodPost, "/api/v2/projects/20/plan", plan, token, "")
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})
}

func TestTaskLeaseHeartbeatV2(t *testing.T) {
	e, err := setupTestEnv()
	require.NoError(t, err)
	token := humaTokenFor(t, &testuser1)

	rec := humaRequest(t, e, http.MethodPost, "/api/v2/tasks/1/leases/heartbeat", "", token, "")
	require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
	assert.Contains(t, rec.Body.String(), `"stale":false`)
	assert.Contains(t, rec.Body.String(), `"pattern":"pkg/models/tasks.go"`)

	rec = humaRequest(t, e, http.MethodPost, "/api/v2/tasks/18/leases/heartbeat", "", token, "")
	assert.Equal(t, http.StatusForbidden, rec.Code, "read-only project")
}
