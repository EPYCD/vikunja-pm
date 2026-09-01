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

type taskReadinessListBody struct {
	Body Paginated[*models.TaskReadiness]
}

// RegisterTaskReadinessRoutes wires the ready queue onto the Huma API.
func RegisterTaskReadinessRoutes(api huma.API) {
	Register(api, huma.Operation{
		OperationID: "projects-views-readiness",
		Summary:     "The ready queue of a kanban view",
		Description: "Returns every open task in one bucket of the view — by default the view's default bucket, where new work waits — with whether it can be claimed right now and, if not, why: " +
			"assigned to someone, blocked by an unfinished task, or owning a path another in-progress task has leased. This is the single server-side answer both agents (`veans list --ready`) and the board read, so they never disagree. " +
			"Not paginated; a bucket is a bounded queue. Requires read access to the project.",
		Method: http.MethodGet,
		Path:   "/projects/{project}/views/{view}/readiness",
		Tags:   []string{"projects"},
	}, projectViewReadiness)
}

func init() { AddRouteRegistrar(RegisterTaskReadinessRoutes) }

func projectViewReadiness(ctx context.Context, in *struct {
	ProjectID int64 `path:"project" doc:"The numeric id of the project."`
	ViewID    int64 `path:"view" doc:"The numeric id of the kanban view."`
	BucketID  int64 `query:"bucket_id" doc:"The bucket to evaluate. Defaults to the view's default bucket."`
}) (*taskReadinessListBody, error) {
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
	view, err := models.GetProjectViewByIDAndProject(s, in.ViewID, in.ProjectID)
	if err != nil {
		return nil, translateDomainError(err)
	}
	items, err := models.GetTaskReadiness(s, a, view, in.BucketID)
	if err != nil {
		return nil, translateDomainError(err)
	}
	return &taskReadinessListBody{Body: NewPaginated(items, int64(len(items)), 1, len(items))}, nil
}
