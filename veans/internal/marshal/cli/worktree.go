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

package cli

import (
	"strconv"

	"github.com/spf13/cobra"

	"code.vikunja.io/veans/internal/marshal/ledger"
	"code.vikunja.io/veans/internal/output"
)

func newWorktreeCmd() *cobra.Command {
	var worker string
	cmd := &cobra.Command{
		Use:   "worktree <task>",
		Short: "The exact worktree command for a story, with a database and port from the pool",
		Long: `Derives the branch and directory from the story id, allocates an unused
database URI and port to the worker, records the checkout so a second worker
resolving to the same path is refused, and prints the commands to run.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := load()
			if err != nil {
				return err
			}
			t, err := resolveTask(cmd.Context(), e, args[0])
			if err != nil {
				return err
			}
			if worker == "" {
				worker = e.Board.Identity
			}
			plan, err := e.PlanWorktree(t, worker)
			if err != nil {
				return err
			}
			return emit(cmd.OutOrStdout(), plan)
		},
	}
	cmd.Flags().StringVar(&worker, "worker", "", "who the checkout belongs to (default: the token's identity)")
	return cmd
}

func newPoolCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "pool",
		Short: "The per-worker database and port allocations",
	}
	list := &cobra.Command{
		Use:   "list",
		Short: "Allocations, local worktrees, board agents and checkout conflicts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := load()
			if err != nil {
				return err
			}
			w, err := e.Workers(cmd.Context())
			if err != nil {
				return err
			}
			if err := emit(cmd.OutOrStdout(), w); err != nil {
				return err
			}
			if len(w.Conflicts) > 0 {
				return output.New(output.CodeConflict, "%d checkout conflict(s)", len(w.Conflicts))
			}
			return nil
		},
	}
	release := &cobra.Command{
		Use:   "release <task-id|worker>",
		Short: "Free a task's or a worker's database and port (done automatically on merge)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := load()
			if err != nil {
				return err
			}
			var freed any
			if id, perr := strconv.ParseInt(args[0], 10, 64); perr == nil {
				freed, err = e.Pool.ReleaseTask(id)
			} else {
				freed, err = e.Pool.ReleaseWorker(args[0])
			}
			if err != nil {
				return err
			}
			e.Log(ledger.Entry{Action: "free", Subject: args[0], Outcome: "ok"})
			return emit(cmd.OutOrStdout(), map[string]any{"released": freed})
		},
	}
	cmd.AddCommand(list, release)
	return cmd
}
