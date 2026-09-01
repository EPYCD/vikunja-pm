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

	"code.vikunja.io/api/pkg/models"
	"code.vikunja.io/api/pkg/web/handler"

	"github.com/danielgtaylor/huma/v2"
)

// RegisterTaskScopeRoutes wires a task's scope — the files and endpoints it
// touches — onto the Huma API. It is a 1:1 sub-resource keyed by the task, so
// there is no create: PUT upserts.
func RegisterTaskScopeRoutes(api huma.API) {
	tags := []string{"tasks"}

	Register(api, huma.Operation{
		OperationID: "tasks-scope-read",
		Summary:     "Get a task's scope",
		Description: "Returns the files the task will edit (paths_owned), the files it reads or affects, the API endpoints it changes and its scope notes. " +
			"A task without a scope returns empty lists rather than 404, so the section can always be rendered and written into. Requires read access to the task.",
		Method: http.MethodGet,
		Path:   "/tasks/{projecttask}/scope",
		Tags:   tags,
	}, taskScopeRead)

	Register(api, huma.Operation{
		OperationID: "tasks-scope-update",
		Summary:     "Set a task's scope",
		Description: "Replaces the task's scope, creating it if none exists. Every path is normalised (leading ./ and / stripped, repeated slashes collapsed) and rejected with 400 if it is empty, longer than 500 characters or contains '..'. " +
			"paths_owned is enforced: claiming the task leases those globs, and if the task is already claimed its leases are re-checked against every other active lease in the project and updated in place, so widening the scope mid-work can fail with 409. " +
			"Everything else is advisory. Requires write access to the task.",
		Method: http.MethodPut,
		Path:   "/tasks/{projecttask}/scope",
		Tags:   tags,
	}, taskScopeUpdate)

	Register(api, huma.Operation{
		OperationID: "tasks-scope-delete",
		Summary:     "Delete a task's scope",
		Description: "Removes the scope and releases any leases derived from it. Requires write access to the task.",
		Method:      http.MethodDelete,
		Path:        "/tasks/{projecttask}/scope",
		Tags:        tags,
	}, taskScopeDelete)
}

func init() { AddRouteRegistrar(RegisterTaskScopeRoutes) }

func taskScopeRead(ctx context.Context, in *struct {
	TaskID int64 `path:"projecttask" doc:"The numeric id of the task."`
}) (*singleBody[models.TaskScope], error) {
	a, err := authFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	scope := &models.TaskScope{TaskID: in.TaskID}
	if _, err := handler.DoReadOne(ctx, scope, a); err != nil {
		return nil, translateDomainError(err)
	}
	return &singleBody[models.TaskScope]{Body: scope}, nil
}

func taskScopeUpdate(ctx context.Context, in *struct {
	TaskID int64 `path:"projecttask" doc:"The numeric id of the task."`
	Body   models.TaskScope
}) (*singleBody[models.TaskScope], error) {
	a, err := authFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	scope := &in.Body
	scope.TaskID = in.TaskID // URL wins over body
	if err := handler.DoUpdate(ctx, scope, a); err != nil {
		return nil, translateDomainError(err)
	}
	return &singleBody[models.TaskScope]{Body: scope}, nil
}

func taskScopeDelete(ctx context.Context, in *struct {
	TaskID int64 `path:"projecttask" doc:"The numeric id of the task."`
}) (*emptyBody, error) {
	a, err := authFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	scope := &models.TaskScope{TaskID: in.TaskID}
	if err := handler.DoDelete(ctx, scope, a); err != nil {
		return nil, translateDomainError(err)
	}
	return &emptyBody{}, nil
}
