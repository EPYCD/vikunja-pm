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

	"github.com/spf13/cobra"

	"code.vikunja.io/veans/internal/client"
)

func newLeasesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "leases",
		Short: "List the paths currently leased by in-progress tasks in the project",
		Long: `Prints every active path lease in the configured project, oldest first,
with the holding task and user embedded — the live answer to "which files
are other agents editing right now". Leases appear when a task with
paths-owned is claimed and disappear when it is done, scrapped, deleted or
released.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			rt, err := loadRuntime()
			if err != nil {
				return err
			}
			leases, err := rt.client.ListProjectLeases(cmd.Context(), rt.cfg.ProjectID)
			if err != nil {
				return err
			}
			if leases == nil {
				leases = []*client.TaskPathLease{}
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(leases)
		},
	}
}

func newReleaseCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "release <id>",
		Short: "Release a task's path leases without changing its status",
		Long: `Drops every lease the task holds so other tasks owning the same paths can
be claimed. The task stays where it is and keeps its assignees — use this
when abandoning work, or to unblock a stale lease left by a crashed session.
Finishing (in-review does not release; done and scrapped do) and deleting a
task release automatically. Idempotent.`,
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
			if err := rt.client.ReleaseTaskLeases(cmd.Context(), id); err != nil {
				return err
			}
			task, err := rt.client.GetTask(cmd.Context(), id)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(task)
		},
	}
}
