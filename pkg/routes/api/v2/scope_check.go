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

// RegisterScopeCheckRoutes wires the diff-versus-scope check onto the Huma API.
func RegisterScopeCheckRoutes(api huma.API) {
	Register(api, huma.Operation{
		OperationID: "projects-scope-check",
		Summary:     "Check changed files against task scopes",
		Description: "Judges a list of changed files against the scopes of the tasks a change references and against every other in-progress task's leases. " +
			"Each file comes back owned (inside a referenced task's paths_owned), affected (only in paths_affected), unscoped, or leased_by_other. " +
			"ok is false when any file is leased by an unreferenced task or, when at least one referenced task declares paths_owned, when any file is unscoped. " +
			"Read-only: nothing is stored. This is what `veans check` and the pull-request check call. Requires read access to the project.",
		Method:        http.MethodPost,
		Path:          "/projects/{project}/scope-check",
		Tags:          []string{"projects"},
		DefaultStatus: http.StatusOK,
	}, projectScopeCheck)
}

func init() { AddRouteRegistrar(RegisterScopeCheckRoutes) }

func projectScopeCheck(ctx context.Context, in *struct {
	ProjectID int64 `path:"project" doc:"The numeric id of the project."`
	Body      models.ScopeCheckRequest
}) (*singleBody[models.ScopeCheckResult], error) {
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
	res, err := models.CheckScope(s, in.ProjectID, &in.Body)
	if err != nil {
		return nil, translateDomainError(err)
	}
	return &singleBody[models.ScopeCheckResult]{Body: res}, nil
}
