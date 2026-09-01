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
	"code.vikunja.io/veans/internal/output"
)

// scopeFlags is shared by `scope` and `create`: both write the same four
// fields, and both need to know which flags were actually passed so an
// omitted list leaves the stored one alone.
type scopeFlags struct {
	owned      []string
	affected   []string
	endpoints  []string
	notes      string
	ownedSet   bool
	affectSet  bool
	endpSet    bool
	notesSet   bool
	clearScope bool
}

func (f *scopeFlags) bind(cmd *cobra.Command) {
	cmd.Flags().StringSliceVar(&f.owned, "paths-owned", nil, "repository-relative globs the task will EDIT (repeatable; leased on claim, e.g. pkg/models/**)")
	cmd.Flags().StringSliceVar(&f.affected, "paths-affected", nil, "globs the task reads or depends on but does not edit (repeatable; advisory)")
	cmd.Flags().StringSliceVar(&f.endpoints, "endpoint", nil, "API surface the task adds or changes, e.g. 'POST /api/v2/tasks/{id}/claim' (repeatable; advisory)")
	cmd.Flags().StringVar(&f.notes, "scope-notes", "", "free-form scope notes, typically what is out of scope (HTML)")
}

func (f *scopeFlags) read(cmd *cobra.Command) {
	f.ownedSet = cmd.Flags().Changed("paths-owned")
	f.affectSet = cmd.Flags().Changed("paths-affected")
	f.endpSet = cmd.Flags().Changed("endpoint")
	f.notesSet = cmd.Flags().Changed("scope-notes")
}

func (f *scopeFlags) any() bool {
	return f.ownedSet || f.affectSet || f.endpSet || f.notesSet
}

// apply folds the passed flags into base and returns the scope to PUT.
func (f *scopeFlags) apply(base *client.TaskScope) *client.TaskScope {
	out := &client.TaskScope{
		PathsOwned:    []string{},
		PathsAffected: []string{},
		Endpoints:     []string{},
	}
	if base != nil {
		if base.PathsOwned != nil {
			out.PathsOwned = base.PathsOwned
		}
		if base.PathsAffected != nil {
			out.PathsAffected = base.PathsAffected
		}
		if base.Endpoints != nil {
			out.Endpoints = base.Endpoints
		}
		out.Notes = base.Notes
	}
	if f.ownedSet {
		out.PathsOwned = f.owned
	}
	if f.affectSet {
		out.PathsAffected = f.affected
	}
	if f.endpSet {
		out.Endpoints = f.endpoints
	}
	if f.notesSet {
		out.Notes = f.notes
	}
	return out
}

func newScopeCmd() *cobra.Command {
	f := &scopeFlags{}
	cmd := &cobra.Command{
		Use:   "scope <id>",
		Short: "Show or set a task's scope: the files it owns, touches and the endpoints it changes",
		Long: `Without flags, prints the task's scope. With flags, replaces the given
fields and prints the result; fields you don't pass are kept.

paths-owned is the only enforced part: claiming the task leases those globs
for the project, and any other task whose paths-owned overlap an active lease
cannot be claimed until the holder is done or released. Everything else is
context for the agent working the task.

Paths are repository-relative. Globs: * matches within a segment, ** across
segments; a bare directory such as pkg/models covers its whole subtree.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := loadRuntime()
			if err != nil {
				return err
			}
			f.read(cmd)
			id, err := rt.resolveTaskID(cmd.Context(), args[0])
			if err != nil {
				return err
			}

			if f.clearScope {
				if f.any() {
					return output.New(output.CodeValidation, "--clear cannot be combined with other scope flags")
				}
				if err := rt.client.DeleteTaskScope(cmd.Context(), id); err != nil {
					return err
				}
			}

			var scope *client.TaskScope
			if f.any() {
				scope, err = setScope(cmd.Context(), rt, id, f)
			} else {
				scope, err = rt.client.GetTaskScope(cmd.Context(), id)
			}
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(scope)
		},
	}
	f.bind(cmd)
	cmd.Flags().BoolVar(&f.clearScope, "clear", false, "remove the scope and any leases derived from it")
	return cmd
}

// setScope merges the flags over the stored scope and writes it back. A
// CONFLICT means the task already holds leases and the new paths_owned
// collide with another task's — the caller must narrow them or wait.
func setScope(ctx context.Context, rt *runtime, id int64, f *scopeFlags) (*client.TaskScope, error) {
	current, err := rt.client.GetTaskScope(ctx, id)
	if err != nil {
		return nil, err
	}
	return rt.client.PutTaskScope(ctx, id, f.apply(current))
}
