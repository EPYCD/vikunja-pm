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

func newHeartbeatCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "heartbeat <id>",
		Short: "Mark a task's path leases as still active",
		Long: `Refreshes the task's leases so they are not flagged stale. Every claim,
update and comment you make on the task counts as activity already; run this
when you work for a long time without touching the board.`,
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
			leases, err := rt.client.HeartbeatTaskLeases(cmd.Context(), id)
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
