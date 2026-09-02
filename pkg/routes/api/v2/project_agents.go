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

type projectAgentListBody struct {
	Body Paginated[*models.ProjectAgent]
}

// RegisterProjectAgentRoutes wires the who-is-working-here listing.
func RegisterProjectAgentRoutes(api huma.API) {
	Register(api, huma.Operation{
		OperationID: "projects-agents",
		Summary:     "Who is working in a project",
		Description: "Every user assigned to an open task or holding a path lease in the project, with open-task and lease counts. Bots come first and carry the human who owns them, so a board can say whose agent holds what. Not paginated. Requires read access to the project.",
		Method:      http.MethodGet,
		Path:        "/projects/{project}/agents",
		Tags:        []string{"projects"},
	}, projectAgents)
}

func init() { AddRouteRegistrar(RegisterProjectAgentRoutes) }

func projectAgents(ctx context.Context, in *struct {
	ProjectID int64 `path:"project" doc:"The numeric id of the project."`
}) (*projectAgentListBody, error) {
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
	items, err := models.GetProjectAgents(s, in.ProjectID)
	if err != nil {
		return nil, translateDomainError(err)
	}
	return &projectAgentListBody{Body: NewPaginated(items, int64(len(items)), 1, len(items))}, nil
}
