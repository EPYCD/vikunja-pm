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
package commands

import (
	"encoding/json"
	"io"
	"os"

	"github.com/spf13/cobra"

	"code.vikunja.io/veans/internal/client"
	"code.vikunja.io/veans/internal/output"
)

func newPlanCmd() *cobra.Command {
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "plan <file.json|->",
		Short: "Lint a decomposition and create all of its tasks in one go",
		Long: `Reads a plan — a JSON object with a "tasks" array — lints it as a set and
creates every task, relation and scope in one transaction when it is clean.

  {"tasks": [
    {"key": "api", "title": "atomic claim", "priority": 3,
     "scope": {"paths_owned": ["pkg/models/task_claim.go"], "endpoints": ["POST /api/v2/tasks/{id}/claim"]}},
    {"key": "ui", "title": "board badges", "blocked_by": ["api"],
     "scope": {"paths_owned": ["frontend/src/components/tasks/**"]}},
    {"key": "epic", "title": "claiming"},
    {"key": "docs", "title": "docs", "parent_key": "epic", "follows": ["ui"]}
  ]}

Keys only exist inside the plan. Errors (duplicate or unknown keys, cycles,
invalid paths) print the findings and exit CONFLICT without creating
anything; warnings (missing scope, overlapping paths with no ordering,
overlap with an open task on the board) are printed and the plan is created
anyway. Use --dry-run to only lint. Bare scope paths are prefixed with the
repository from .veans.yml when one is set.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := loadRuntime()
			if err != nil {
				return err
			}
			var raw []byte
			if args[0] == "-" {
				raw, err = io.ReadAll(os.Stdin)
			} else {
				raw, err = os.ReadFile(args[0])
			}
			if err != nil {
				return output.Wrap(output.CodeValidation, err, "read plan: %v", err)
			}
			var plan client.TaskPlan
			if err := json.Unmarshal(raw, &plan); err != nil {
				return output.Wrap(output.CodeValidation, err, "parse plan: %v", err)
			}
			if len(plan.Tasks) == 0 {
				return output.New(output.CodeValidation, "the plan has no tasks")
			}
			plan.DryRun = plan.DryRun || dryRun
			for _, t := range plan.Tasks {
				if t.Scope != nil {
					t.Scope.PathsOwned = withRepoPrefix(rt.cfg.Repository, t.Scope.PathsOwned)
					t.Scope.PathsAffected = withRepoPrefix(rt.cfg.Repository, t.Scope.PathsAffected)
				}
			}
			res, err := rt.client.ApplyPlan(cmd.Context(), rt.cfg.ProjectID, &plan)
			if err != nil {
				return err
			}
			if err := json.NewEncoder(cmd.OutOrStdout()).Encode(res); err != nil {
				return err
			}
			if !res.OK {
				return output.New(output.CodeConflict, "the plan has errors; nothing was created — see stdout for the findings")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "lint only, create nothing")
	return cmd
}
