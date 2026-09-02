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
	"code.vikunja.io/api/pkg/events"
	"code.vikunja.io/api/pkg/models"

	"github.com/danielgtaylor/huma/v2"
)

// RegisterTaskPlanRoutes wires bulk decomposition onto the Huma API.
func RegisterTaskPlanRoutes(api huma.API) {
	Register(api, huma.Operation{
		OperationID: "projects-plan",
		Summary:     "Lint and create a decomposition of tasks in one request",
		Description: "Takes a whole plan — tasks addressed by plan-local keys, their parent, blocked_by and follows relations and their scopes — lints it as a set and creates everything in one transaction when it is clean. " +
			"Errors (duplicate or unknown keys, self references, dependency or parent cycles, invalid paths) stop creation and are returned with ok=false; warnings (tasks without paths_owned, overlapping owned paths with no ordering between them, overlap with an open task already on the board) are returned alongside the created tasks. " +
			"dry_run only lints. Always answers 200 with the findings; read ok and created. Requires write access to the project.",
		Method:        http.MethodPost,
		Path:          "/projects/{project}/plan",
		Tags:          []string{"projects"},
		DefaultStatus: http.StatusOK,
	}, projectPlan)

	Register(api, huma.Operation{
		OperationID: "projects-plan-export",
		Summary:     "Export the open tasks of a project in plan shape",
		Description: "Returns every open task with its parent, blocked_by and follows relations and its scope, keyed by task identifier (PROJ-12 or #12). " +
			"Edit the file, add tasks that reference the exported keys, and POST it back: keys the plan does not define are resolved against the board, so incremental re-planning never duplicates what already exists. " +
			"Requires read access to the project.",
		Method: http.MethodGet,
		Path:   "/projects/{project}/plan",
		Tags:   []string{"projects"},
	}, projectPlanExport)
}

func init() { AddRouteRegistrar(RegisterTaskPlanRoutes) }

func projectPlanExport(ctx context.Context, in *struct {
	ProjectID int64 `path:"project" doc:"The numeric id of the project."`
}) (*singleBody[models.ExportedPlan], error) {
	a, err := authFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	s := db.NewSession()
	defer s.Close()

	// Non-CRUD read, so the permission check is the handler's job.
	project := &models.Project{ID: in.ProjectID}
	can, _, err := project.CanRead(s, a)
	if err != nil {
		return nil, translateDomainError(err)
	}
	if !can {
		return nil, huma.Error403Forbidden("forbidden")
	}
	plan, err := models.ExportTaskPlan(s, in.ProjectID)
	if err != nil {
		return nil, translateDomainError(err)
	}
	return &singleBody[models.ExportedPlan]{Body: plan}, nil
}

func projectPlan(ctx context.Context, in *struct {
	ProjectID int64 `path:"project" doc:"The numeric id of the project."`
	Body      models.TaskPlan
}) (*singleBody[models.PlanResult], error) {
	a, err := authFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	s := db.NewSession()
	defer s.Close()
	if err := s.Begin(); err != nil {
		return nil, err
	}

	// Non-CRUD action, so the permission check is the handler's job.
	project := &models.Project{ID: in.ProjectID}
	can, err := project.CanWrite(s, a)
	if err != nil {
		_ = s.Rollback()
		return nil, translateDomainError(err)
	}
	if !can {
		_ = s.Rollback()
		return nil, huma.Error403Forbidden("forbidden")
	}
	res, err := models.ApplyTaskPlan(s, a, in.ProjectID, &in.Body)
	if err != nil {
		_ = s.Rollback()
		events.CleanupPending(s)
		return nil, translateDomainError(err)
	}
	if err := s.Commit(); err != nil {
		events.CleanupPending(s)
		return nil, translateDomainError(err)
	}
	events.DispatchPending(ctx, s)
	return &singleBody[models.PlanResult]{Body: res}, nil
}
