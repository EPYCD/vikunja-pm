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
	"fmt"
	"net/http"

	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/models"
	"code.vikunja.io/api/pkg/web/handler"

	"github.com/danielgtaylor/huma/v2"
)

type taskPathLeaseListBody struct {
	Body Paginated[*models.TaskPathLease]
}

// RegisterTaskPathLeaseRoutes wires path leases onto the Huma API: a project-
// wide listing (what is being edited right now, by whom) and an explicit
// release on a task.
func RegisterTaskPathLeaseRoutes(api huma.API) {
	Register(api, huma.Operation{
		OperationID: "projects-leases-list",
		Summary:     "List the active path leases of a project",
		Description: "Returns every path pattern currently held by an in-progress task in the project, oldest first, with the holding task and user embedded. " +
			"This is the live answer to \"which files are agents editing right now\". Leases are created when a task with paths_owned in its scope is claimed and released when it is done, scrapped, deleted or explicitly released. " +
			"Not paginated: a project has at most a handful of tasks in flight. Requires read access to the project.",
		Method: http.MethodGet,
		Path:   "/projects/{project}/leases",
		Tags:   []string{"projects"},
	}, projectLeasesList)

	Register(api, huma.Operation{
		OperationID: "tasks-leases-release",
		Summary:     "Release a task's path leases",
		Description: "Drops every lease the task holds without changing its status or assignees — for an agent abandoning work, or a human unblocking a stale lease left by a crashed session. " +
			"Idempotent: releasing a task that holds nothing succeeds. Requires write access to the task.",
		Method: http.MethodDelete,
		Path:   "/tasks/{projecttask}/leases",
		Tags:   []string{"tasks"},
	}, taskLeasesRelease)
}

func init() { AddRouteRegistrar(RegisterTaskPathLeaseRoutes) }

func projectLeasesList(ctx context.Context, in *struct {
	ProjectID int64 `path:"project" doc:"The numeric id of the project."`
}) (*taskPathLeaseListBody, error) {
	a, err := authFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	result, _, total, err := handler.DoReadAll(ctx, &models.TaskPathLease{ProjectID: in.ProjectID}, a, "", 1, 0)
	if err != nil {
		return nil, translateDomainError(err)
	}
	items, ok := result.([]*models.TaskPathLease)
	if !ok {
		return nil, fmt.Errorf("taskPathLease.ReadAll returned unexpected type %T", result)
	}
	return &taskPathLeaseListBody{Body: NewPaginated(items, total, 1, len(items))}, nil
}

func taskLeasesRelease(ctx context.Context, in *struct {
	TaskID int64 `path:"projecttask" doc:"The numeric id of the task."`
}) (*emptyBody, error) {
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
	task := &models.Task{ID: in.TaskID}
	can, err := task.CanWrite(s, a)
	if err != nil {
		_ = s.Rollback()
		return nil, translateDomainError(err)
	}
	if !can {
		_ = s.Rollback()
		return nil, huma.Error403Forbidden("forbidden")
	}
	if err := models.ReleaseTaskPathLeases(s, a, in.TaskID); err != nil {
		_ = s.Rollback()
		return nil, translateDomainError(err)
	}
	if err := s.Commit(); err != nil {
		return nil, translateDomainError(err)
	}
	return &emptyBody{}, nil
}
