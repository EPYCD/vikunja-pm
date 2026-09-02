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

// Package cli is the marshal command. Like veans, every command prints JSON
// on stdout and a JSON error envelope on stderr; humans run init, setup and
// serve once, agents and CI run the rest.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"code.vikunja.io/veans/internal/client"
	"code.vikunja.io/veans/internal/marshal/engine"
	"code.vikunja.io/veans/internal/output"
)

// Root builds the command tree.
func Root(version string) *cobra.Command {
	root := &cobra.Command{
		Use:   "marshal",
		Short: "The repo-aware layer over a Workman board: spec references, ownership, gate receipts, worktrees",
		Long: `Marshal keeps a Workman board honest against the repository it coordinates:
references resolve from the spec at read time, file claims are checked against
branches and CODEOWNERS, "done" needs a CI receipt, and every worker gets a
worktree, a database and a port. It runs beside the board (marshal serve), as a
CLI, and as an MCP server for agents (marshal mcp).`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version,
	}
	root.AddCommand(newInitCmd())
	root.AddCommand(newSetupCmd())
	root.AddCommand(newRefsCmd())
	root.AddCommand(newHealthCmd())
	root.AddCommand(newChokepointsCmd())
	root.AddCommand(newOpenCmd())
	root.AddCommand(newReconcileCmd())
	root.AddCommand(newWorktreeCmd())
	root.AddCommand(newPoolCmd())
	root.AddCommand(newClaimsCmd())
	root.AddCommand(newReceiptCmd())
	root.AddCommand(newLedgerCmd())
	root.AddCommand(newNotifyCmd())
	root.AddCommand(newServeCmd())
	root.AddCommand(newMCPCmd(version))
	return root
}

// Execute runs the command and renders errors as the JSON envelope.
func Execute(version string) int {
	root := Root(version)
	if err := root.Execute(); err != nil {
		var oe *output.Error
		if !errors.As(err, &oe) {
			oe = output.Wrap(output.CodeUnknown, err, "%v", err)
		}
		if encErr := json.NewEncoder(os.Stderr).Encode(oe); encErr != nil {
			fmt.Fprintln(os.Stderr, oe.Error())
		}
		return 1
	}
	return 0
}

func load() (*engine.Engine, error) {
	return engine.Load("")
}

func emit(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// resolveTask accepts PROJ-NN, #NN, NN (index) or id:NN.
func resolveTask(ctx context.Context, e *engine.Engine, raw string) (*client.Task, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, output.New(output.CodeValidation, "a task reference is required")
	}
	if strings.HasPrefix(raw, "id:") {
		id, err := strconv.ParseInt(raw[3:], 10, 64)
		if err != nil {
			return nil, output.Wrap(output.CodeValidation, err, "invalid task id %q", raw)
		}
		return e.Board.Client.GetTask(ctx, id)
	}
	ref := strings.TrimPrefix(raw, "#")
	if i := strings.LastIndex(ref, "-"); i >= 0 {
		ref = ref[i+1:]
	}
	index, err := strconv.ParseInt(ref, 10, 64)
	if err != nil {
		return nil, output.Wrap(output.CodeValidation, err, "invalid task reference %q", raw)
	}
	var t client.Task
	if err := e.Board.Client.Do(ctx, "GET", fmt.Sprintf("/projects/%d/tasks/by-index/%d", e.Board.Cfg.ProjectID, index), nil, nil, &t); err != nil {
		return nil, err
	}
	return e.Board.Client.GetTask(ctx, t.ID)
}
