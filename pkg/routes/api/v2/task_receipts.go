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

	"code.vikunja.io/api/pkg/models"
	"code.vikunja.io/api/pkg/web/handler"

	"github.com/danielgtaylor/huma/v2"
)

func init() { AddRouteRegistrar(RegisterTaskReceiptRoutes) }

type taskReceiptListBody struct {
	Body Paginated[*models.TaskReceipt]
}

// RegisterTaskReceiptRoutes: CI's gate receipts on a task. Create and list
// only — a receipt is never edited or removed.
func RegisterTaskReceiptRoutes(api huma.API) {
	tags := []string{"tasks"}

	Register(api, huma.Operation{
		OperationID: "tasks-receipts-list",
		Summary:     "List a task's gate receipts",
		Description: "Every receipt CI posted for the task, newest first. A receipt records the commit, the branch, each gate's result and duration, whether the API docs had to be and were regenerated, the CI run link and whether the commit is on the merged branch. Requires read access to the task.",
		Method:      http.MethodGet,
		Path:        "/tasks/{projecttask}/receipts",
		Tags:        tags,
	}, taskReceiptsList)

	Register(api, huma.Operation{
		OperationID: "tasks-receipts-create",
		Summary:     "Post a gate receipt",
		Description: "Only the project's receipt bot (projects.receipt_bot_id) may call this; anyone else gets 403, including the task's assignee. Failing runs are posted too. The server computes `passed`: every gate passed and, when docs_api_required, the docs were regenerated. While a project names a receipt bot, a task cannot be marked done without a merged, passing receipt.",
		Method:      http.MethodPost,
		Path:        "/tasks/{projecttask}/receipts",
		Tags:        tags,
	}, taskReceiptsCreate)
}

func taskReceiptsList(ctx context.Context, in *struct {
	TaskID int64 `path:"projecttask"`
	ListParams
}) (*taskReceiptListBody, error) {
	a, err := authFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	r := &models.TaskReceipt{TaskID: in.TaskID}
	result, count, total, err := handler.DoReadAll(ctx, r, a, in.Q, in.Page, in.PerPage)
	if err != nil {
		return nil, translateDomainError(err)
	}
	items, ok := result.([]*models.TaskReceipt)
	if !ok {
		return nil, fmt.Errorf("receipts.ReadAll returned unexpected type %T", result)
	}
	_ = count
	return &taskReceiptListBody{Body: NewPaginated(items, total, in.Page, in.PerPage)}, nil
}

func taskReceiptsCreate(ctx context.Context, in *struct {
	TaskID int64 `path:"projecttask"`
	Body   models.TaskReceipt
}) (*singleBody[models.TaskReceipt], error) {
	a, err := authFromCtx(ctx)
	if err != nil {
		return nil, err
	}
	in.Body.TaskID = in.TaskID
	if err := handler.DoCreate(ctx, &in.Body, a); err != nil {
		return nil, translateDomainError(err)
	}
	return &singleBody[models.TaskReceipt]{Body: &in.Body}, nil
}
