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
	"strings"

	"github.com/spf13/cobra"

	"code.vikunja.io/veans/internal/client"
	"code.vikunja.io/veans/internal/marshal/ledger"
	"code.vikunja.io/veans/internal/marshal/pathpattern"
	"code.vikunja.io/veans/internal/output"
)

// ownsPrefix is the label convention the board used before typed scopes.
const ownsPrefix = "owns:"

type importRow struct {
	TaskID     int64    `json:"task_id"`
	Identifier string   `json:"identifier"`
	Labels     []string `json:"labels"`
	Paths      []string `json:"paths"`
	Invalid    []string `json:"invalid,omitempty"`
	Existing   []string `json:"existing,omitempty"`
	Applied    bool     `json:"applied"`
	Error      string   `json:"error,omitempty"`
}

func newClaimsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "claims",
		Short: "File claims: import the owns: labels into typed scopes",
	}
	var apply, dropLabels bool
	imp := &cobra.Command{
		Use:   "import",
		Short: "Turn every owns:<path> label into paths_owned on its task (dry run unless --apply)",
		Long: `Reads each open task's owns: labels, validates them as path patterns, and
writes them as the task's paths_owned. Existing scope paths are kept; the
union is written. Labels that are not valid paths ("deployment config") are
reported for a human to resolve and never guessed. With --drop-labels the
imported labels are removed after a successful write.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := load()
			if err != nil {
				return err
			}
			snap, err := e.Board.Snapshot(cmd.Context())
			if err != nil {
				return err
			}
			rows := []importRow{}
			invalidTotal := 0
			for _, t := range snap.Tasks {
				row := importRow{TaskID: t.ID, Identifier: t.Identifier}
				var labels []*client.Label
				for _, l := range t.Labels {
					if strings.HasPrefix(l.Title, ownsPrefix) {
						labels = append(labels, l)
						row.Labels = append(row.Labels, l.Title)
						p, err := pathpattern.Normalize(strings.TrimPrefix(l.Title, ownsPrefix))
						if err != nil {
							row.Invalid = append(row.Invalid, l.Title)
							continue
						}
						row.Paths = append(row.Paths, p)
					}
				}
				if len(labels) == 0 {
					continue
				}
				invalidTotal += len(row.Invalid)
				if t.Scope != nil {
					row.Existing = t.Scope.PathsOwned
				}
				if apply && len(row.Paths) > 0 {
					scope := &client.TaskScope{PathsOwned: union(row.Existing, row.Paths), PathsAffected: []string{}, Endpoints: []string{}}
					if t.Scope != nil {
						scope.PathsAffected = t.Scope.PathsAffected
						scope.Endpoints = t.Scope.Endpoints
						scope.Notes = t.Scope.Notes
					}
					if _, err := e.Board.Client.PutTaskScope(cmd.Context(), t.ID, scope); err != nil {
						row.Error = err.Error()
					} else {
						row.Applied = true
						e.Log(ledger.Entry{Action: "import", TaskID: t.ID, Subject: strings.Join(row.Paths, ","), Outcome: "ok"})
						if dropLabels {
							for _, l := range labels {
								if !contains(row.Invalid, l.Title) {
									_ = e.Board.Client.RemoveLabelFromTask(cmd.Context(), t.ID, l.ID)
								}
							}
						}
					}
				}
				rows = append(rows, row)
			}
			if err := emit(cmd.OutOrStdout(), map[string]any{"apply": apply, "tasks": rows, "invalid": invalidTotal}); err != nil {
				return err
			}
			if invalidTotal > 0 {
				return output.New(output.CodeConflict, "%d owns: label(s) are not paths and need a human", invalidTotal)
			}
			return nil
		},
	}
	imp.Flags().BoolVar(&apply, "apply", false, "write the scopes (default: report only)")
	imp.Flags().BoolVar(&dropLabels, "drop-labels", false, "remove imported owns: labels after writing")
	cmd.AddCommand(imp)
	return cmd
}

func union(a, b []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range append(append([]string{}, a...), b...) {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
