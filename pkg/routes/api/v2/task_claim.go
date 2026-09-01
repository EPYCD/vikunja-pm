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

package apiv2

import (
	"context"
	"net/http"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/models"

	"github.com/danielgtaylor/huma/v2"
)

// RegisterTaskClaimRoutes wires the atomic task claim onto the Huma API.
//
// This is a non-CRUD action, so the handler owns both the permission check and
// the transaction: TaskBucket.CanUpdate gates it (write on the task plus the
// bucket resolving under the task's project and the given view), and the whole
// check-move-assign runs in one session so it commits or rolls back as a unit.
func RegisterTaskClaimRoutes(api huma.API) {
	Register(api, huma.Operation{
		OperationID: "tasks-claim",
		Summary:     "Claim a task",
		Description: "Takes the task for the authenticated user if nobody else holds it: in one transaction it verifies the task has no other assignee " +
			"(and, when expected_bucket_id is given, that it is still in that bucket), moves it into bucket_id on the given view and assigns the caller. " +
			"Two callers racing for the same task get exactly one winner; the loser receives 409 with error code 4034 (already claimed) or 4035 (left the expected bucket). " +
			"Re-claiming a task you already hold succeeds without changing anything. Requires write access to the task; link shares cannot claim. " +
			"Moving into a bucket that is at its task limit is rejected with 412, and moving into the view's done bucket marks the task done, exactly as the bucket-move endpoint does.",
		Method:        http.MethodPost,
		Path:          "/tasks/{projecttask}/claim",
		Tags:          []string{"tasks"},
		DefaultStatus: http.StatusOK,
	}, tasksClaim)
}

func init() { AddRouteRegistrar(RegisterTaskClaimRoutes) }

func tasksClaim(ctx context.Context, in *struct {
	TaskID int64 `path:"projecttask" doc:"The numeric id of the task to claim."`
	Body   models.TaskClaim
}) (*singleBody[models.Task], error) {
	a, err := authFromCtx(ctx)
	if err != nil {
		return nil, err
	}

	s := db.NewSession()
	defer s.Close()
	if err := s.Begin(); err != nil {
		return nil, err
	}

	// The bucket check needs the project the view belongs to, which is the
	// task's project; a bucket from another project is refused there.
	task, err := models.GetTaskByIDSimple(s, in.TaskID)
	if err != nil {
		_ = s.Rollback()
		return nil, translateDomainError(err)
	}
	gate := &models.TaskBucket{
		TaskID:        in.TaskID,
		ProjectViewID: in.Body.ProjectViewID,
		BucketID:      in.Body.BucketID,
		ProjectID:     task.ProjectID,
	}
	can, err := gate.CanUpdate(s, a)
	if err != nil {
		_ = s.Rollback()
		return nil, translateDomainError(err)
	}
	if !can {
		_ = s.Rollback()
		return nil, huma.Error403Forbidden("forbidden")
	}

	claim := &in.Body
	claim.TaskID = in.TaskID // URL wins over body
	claimed, err := models.ClaimTask(s, a, claim)
	if err != nil {
		_ = s.Rollback()
		return nil, translateDomainError(err)
	}
	if err := s.Commit(); err != nil {
		return nil, translateDomainError(err)
	}
	return &singleBody[models.Task]{Body: claimed}, nil
}
