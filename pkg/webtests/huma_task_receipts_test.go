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
	"code.vikunja.io/api/pkg/user"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTaskReceiptsV2 covers POST/GET /tasks/{projecttask}/receipts and the
// done gate they feed. Project 1 (task 1) is owned by testuser1.
func TestTaskReceiptsV2(t *testing.T) {
	const green = `{"commit_sha":"abc1234def","branch":"e5.3-scheduler","gates":[{"name":"typecheck","status":"passed","duration_ms":1000},{"name":"lint","status":"passed","duration_ms":500},{"name":"test","status":"passed","duration_ms":20000},{"name":"build","status":"passed","duration_ms":40000}],"docs_api_required":false,"ci_run_url":"https://ci.example/run/1","merged":true,"merge_sha":"0123abcd"}`

	receiptBot := func(t *testing.T) *user.User {
		t.Helper()
		s := db.NewSession()
		defer s.Close()
		bot := &user.User{Username: "bot-ci", Name: "CI", BotOwnerID: 1, Status: user.StatusActive}
		_, err := s.Insert(bot)
		require.NoError(t, err)
		_, err = s.ID(1).Cols("receipt_bot_id").Update(&models.Project{ReceiptBotID: bot.ID})
		require.NoError(t, err)
		require.NoError(t, s.Commit())
		return bot
	}

	t.Run("the receipt bot posts, humans and other bots cannot", func(t *testing.T) {
		e, err := setupTestEnv()
		require.NoError(t, err)
		bot := receiptBot(t)

		rec := humaRequest(t, e, http.MethodPost, "/api/v2/tasks/1/receipts", green, humaTokenFor(t, &testuser1), "")
		assert.Equal(t, http.StatusForbidden, rec.Code, "body: %s", rec.Body.String())

		rec = humaRequest(t, e, http.MethodPost, "/api/v2/tasks/1/receipts", green, humaTokenFor(t, bot), "")
		require.Equal(t, http.StatusCreated, rec.Code, "body: %s", rec.Body.String())
		var created models.TaskReceipt
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
		assert.True(t, created.Passed)
		assert.Equal(t, int64(1), created.ProjectID)
		require.NotNil(t, created.PostedBy)
		assert.Equal(t, bot.ID, created.PostedBy.ID)

		rec = humaRequest(t, e, http.MethodGet, "/api/v2/tasks/1/receipts", "", humaTokenFor(t, &testuser1), "")
		require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
		var list struct {
			Items []models.TaskReceipt `json:"items"`
			Total int64                `json:"total"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
		assert.Equal(t, int64(1), list.Total)
		require.Len(t, list.Items, 1)
		assert.Equal(t, "abc1234def", list.Items[0].CommitSHA)
	})

	t.Run("a malformed receipt is a 400 with the reason", func(t *testing.T) {
		e, err := setupTestEnv()
		require.NoError(t, err)
		bot := receiptBot(t)
		rec := humaRequest(t, e, http.MethodPost, "/api/v2/tasks/1/receipts", `{"commit_sha":"zzzzzzzz","gates":[{"name":"test","status":"passed"}]}`, humaTokenFor(t, bot), "")
		assert.Equal(t, http.StatusBadRequest, rec.Code, "body: %s", rec.Body.String())
		assert.Contains(t, rec.Body.String(), "4042")
	})

	t.Run("done is refused without a receipt and allowed with one", func(t *testing.T) {
		e, err := setupTestEnv()
		require.NoError(t, err)
		bot := receiptBot(t)
		token := humaTokenFor(t, &testuser1)

		rec := humaRequest(t, e, http.MethodPatch, "/api/v2/tasks/1", `{"done":true}`, token, "application/merge-patch+json")
		assert.Equal(t, http.StatusConflict, rec.Code, "body: %s", rec.Body.String())
		assert.Contains(t, rec.Body.String(), "4039")

		rec = humaRequest(t, e, http.MethodPost, "/api/v2/tasks/1/receipts", green, humaTokenFor(t, bot), "")
		require.Equal(t, http.StatusCreated, rec.Code, "body: %s", rec.Body.String())

		rec = humaRequest(t, e, http.MethodPatch, "/api/v2/tasks/1", `{"done":true}`, token, "application/merge-patch+json")
		assert.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
		db.AssertExists(t, "tasks", map[string]interface{}{"id": 1, "done": true}, false)
	})

	t.Run("the claim bucket round-trips through the v2 view update", func(t *testing.T) {
		e, err := setupTestEnv()
		require.NoError(t, err)
		token := humaTokenFor(t, &testuser1)
		rec := humaRequest(t, e, http.MethodPatch, "/api/v2/projects/1/views/4", `{"claim_bucket_id":2}`, token, "application/merge-patch+json")
		require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
		rec = humaRequest(t, e, http.MethodGet, "/api/v2/projects/1/views/4", "", token, "")
		require.Equal(t, http.StatusOK, rec.Code, "body: %s", rec.Body.String())
		var view struct {
			ClaimBucketID int64 `json:"claim_bucket_id"`
			DoneBucketID  int64 `json:"done_bucket_id"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &view))
		assert.Equal(t, int64(2), view.ClaimBucketID)
		assert.Equal(t, int64(3), view.DoneBucketID, "the other special buckets are untouched")
		db.AssertExists(t, "project_views", map[string]interface{}{"id": 4, "claim_bucket_id": 2}, false)
	})

	t.Run("nobody without access can read receipts", func(t *testing.T) {
		e, err := setupTestEnv()
		require.NoError(t, err)
		receiptBot(t)
		// Task 34 lives in project 20, where testuser1 has no access.
		rec := humaRequest(t, e, http.MethodGet, "/api/v2/tasks/34/receipts", "", humaTokenFor(t, &testuser1), "")
		assert.Equal(t, http.StatusForbidden, rec.Code, "body: %s", rec.Body.String())
	})
}
