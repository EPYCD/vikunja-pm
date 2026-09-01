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
	"context"
	"encoding/json"

	"github.com/spf13/cobra"

	"code.vikunja.io/veans/internal/client"
)

func newReadyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ready",
		Short: "The ready queue: every Todo task with whether it can be claimed and, if not, why",
		Long: `Asks the server for the readiness of every open task in the Todo bucket.
Each row carries the task, "ready", and the reasons it is not: "assigned"
(someone holds it), "blocked" (an unfinished blocker, listed under
blocked_by) or "lease_conflict" (one of its paths-owned overlaps a lease held
by an in-progress task, listed under lease_conflicts).

` + "`veans list --ready`" + ` is the same query reduced to the tasks that are ready.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			rt, err := loadRuntime()
			if err != nil {
				return err
			}
			rows, err := readyQueue(cmd.Context(), rt)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(rows)
		},
	}
}

// readyQueue fetches the Todo bucket's readiness. The server does not expand
// buckets on these tasks, so the bucket is stamped on here — every row is in
// Todo by construction — which keeps CurrentBucketID and the status filters
// working on them.
func readyQueue(ctx context.Context, rt *runtime) ([]*client.TaskReadiness, error) {
	rows, err := rt.client.ProjectReadiness(ctx, rt.cfg.ProjectID, rt.cfg.ViewID, rt.cfg.Buckets.Todo)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []*client.TaskReadiness{}
	}
	for _, r := range rows {
		if r.Task != nil {
			r.Task.BucketID = rt.cfg.Buckets.Todo
		}
	}
	return rows, nil
}
