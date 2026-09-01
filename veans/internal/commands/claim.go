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
	"errors"

	"github.com/spf13/cobra"

	"code.vikunja.io/veans/internal/output"
	"code.vikunja.io/veans/internal/status"
)

func newClaimCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "claim <id>",
		Short: "Claim a task: assign the bot, move to In Progress, tag with branch",
		Long: `Atomically takes a task. The server refuses with CONFLICT if another user
already holds it, or if it has left the Todo bucket since you listed it — so
two agents racing for the same task get exactly one winner. Re-claiming a task
you already hold is a no-op.

Pass --force to claim a task that is not in Todo (for example one a human
parked In Review that you have been asked to pick back up). It never
overrides another user's claim.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := loadRuntime()
			if err != nil {
				return err
			}
			id, err := rt.resolveTaskID(cmd.Context(), args[0])
			if err != nil {
				return err
			}

			inProgress, err := status.BucketID(status.InProgress, rt.cfg.Buckets)
			if err != nil {
				return err
			}
			expected := rt.cfg.Buckets.Todo
			if force {
				expected = 0
			}

			// One request does the check, the bucket move and the assignment
			// in a single server-side transaction; a 409 here means we lost
			// the race and must not touch the task further.
			if _, err := rt.client.ClaimTask(cmd.Context(), id, rt.cfg.ViewID, inProgress, expected); err != nil {
				var oe *output.Error
				if errors.As(err, &oe) && oe.Code == output.CodeConflict {
					return output.Wrap(output.CodeConflict, err,
						"cannot claim task %s: %v — it is held by someone else, left Todo, is blocked, or owns a path another task has leased; run `veans ready` and pick another (use --force to claim from another bucket)",
						args[0], err)
				}
				return err
			}

			task, err := rt.client.GetTask(cmd.Context(), id)
			if err != nil {
				return err
			}

			// Tag with the current branch label, if there is one. A label is
			// bookkeeping, not part of the claim, so it stays outside the
			// transaction. Checked against the task first: a claim runs on
			// every start of work, and re-attaching a label the task already
			// carries is rejected by the server.
			if branch := currentGitBranch(cmd.Context()); branch != "" {
				labelTitle := branchLabel(branch)
				if !taskHasLabel(task, labelTitle) {
					l, err := getOrCreateLabelByTitle(cmd.Context(), rt.client, labelTitle)
					if err != nil {
						return err
					}
					if err := rt.client.AddLabelToTask(cmd.Context(), id, l.ID); err != nil {
						return err
					}
					if task, err = rt.client.GetTask(cmd.Context(), id); err != nil {
						return err
					}
				}
			}

			return json.NewEncoder(cmd.OutOrStdout()).Encode(task)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "claim even if the task is not in the Todo bucket (never overrides another user's claim)")
	return cmd
}
