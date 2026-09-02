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

	"code.vikunja.io/veans/internal/marshal/refs"
	"code.vikunja.io/veans/internal/output"
)

func newRefsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "refs",
		Short: "Spec references: resolve, check the board, find pastes",
	}
	var rev string
	cmd.PersistentFlags().StringVar(&rev, "rev", "", "git revision to read the spec at (default: working tree)")

	resolve := &cobra.Command{
		Use:   "resolve <ref>...",
		Short: "Resolve FR-nnn / AD-n / NFR-n / D-n to their text, stamped with provenance",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := load()
			if err != nil {
				return err
			}
			e.SpecRev = rev
			ix, warnings, err := e.Index(cmd.Context())
			if err != nil {
				return err
			}
			found := refs.Extract(strings.Join(args, " "), e.Prefixes())
			if len(found) == 0 {
				return output.New(output.CodeValidation, "no reference with a configured prefix in %q", strings.Join(args, " "))
			}
			res := ix.Resolve(found)
			exit := error(nil)
			for _, r := range res {
				if !r.Found {
					exit = output.New(output.CodeNotFound, "%s does not resolve at %s", r.Ref.ID, ixRev(ix))
				}
			}
			if err := emit(cmd.OutOrStdout(), map[string]any{"rev": ix.Rev, "resolutions": res, "warnings": warnings}); err != nil {
				return err
			}
			return exit
		},
	}

	check := &cobra.Command{
		Use:   "check [task]",
		Short: "Broken references, verbatim spec pastes and unlinked tasks across the board (or one task)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := load()
			if err != nil {
				return err
			}
			e.SpecRev = rev
			if len(args) == 1 {
				t, err := resolveTask(cmd.Context(), e, args[0])
				if err != nil {
					return err
				}
				res, err := e.ResolveTask(cmd.Context(), t)
				if err != nil {
					return err
				}
				if err := emit(cmd.OutOrStdout(), res); err != nil {
					return err
				}
				if res.Broken > 0 || len(res.Pastes) > 0 {
					return output.New(output.CodeConflict, "%d broken reference(s), %d paste(s)", res.Broken, len(res.Pastes))
				}
				return nil
			}
			snap, err := e.Board.Snapshot(cmd.Context())
			if err != nil {
				return err
			}
			rep, err := e.References(cmd.Context(), snap)
			if err != nil {
				return err
			}
			if err := emit(cmd.OutOrStdout(), rep); err != nil {
				return err
			}
			if len(rep.Broken) > 0 || len(rep.Pastes) > 0 {
				return output.New(output.CodeConflict, "%d broken reference(s), %d paste(s)", len(rep.Broken), len(rep.Pastes))
			}
			return nil
		},
	}

	index := &cobra.Command{
		Use:   "index",
		Short: "List every anchor the configured sources define",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := load()
			if err != nil {
				return err
			}
			e.SpecRev = rev
			ix, warnings, err := e.Index(cmd.Context())
			if err != nil {
				return err
			}
			return emit(cmd.OutOrStdout(), map[string]any{"rev": ix.Rev, "anchors": ix.Anchors, "files": ix.Files, "warnings": warnings})
		},
	}
	cmd.AddCommand(resolve, check, index)
	return cmd
}

func ixRev(ix *refs.Index) string {
	if ix.Rev == "" {
		return "the working tree"
	}
	return ix.Rev
}

func newHealthCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "health",
		Short: "The graph invariants: every story claims files, no unordered overlap, no blocked cycle",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := load()
			if err != nil {
				return err
			}
			snap, err := e.Board.Snapshot(cmd.Context())
			if err != nil {
				return err
			}
			h := e.Health(snap)
			if err := emit(cmd.OutOrStdout(), h); err != nil {
				return err
			}
			if !h.OK {
				return output.New(output.CodeConflict, "%d collision(s), %d cycle(s), %d finding(s)", h.Collisions, h.Cycles, len(h.Findings))
			}
			return nil
		},
	}
}

func newChokepointsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "chokepoints",
		Short: "The CODEOWNERS files and who is queued on each",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := load()
			if err != nil {
				return err
			}
			snap, err := e.Board.Snapshot(cmd.Context())
			if err != nil {
				return err
			}
			rep, err := e.Chokepoints(snap)
			if err != nil {
				return err
			}
			return emit(cmd.OutOrStdout(), rep)
		},
	}
}

func newOpenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "open <path>",
		Short: "Is anything open on this path? Declared scopes and live leases that cover it",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := load()
			if err != nil {
				return err
			}
			snap, err := e.Board.Snapshot(cmd.Context())
			if err != nil {
				return err
			}
			holders, err := e.OpenOnPath(snap, args[0])
			if err != nil {
				return err
			}
			return emit(cmd.OutOrStdout(), map[string]any{"path": args[0], "holders": holders})
		},
	}
}

func newReconcileCmd() *cobra.Command {
	var base, branch string
	cmd := &cobra.Command{
		Use:   "reconcile",
		Short: "Validate every claimed task's branch against its claim; flag quiet branches",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := load()
			if err != nil {
				return err
			}
			snap, err := e.Board.Snapshot(cmd.Context())
			if err != nil {
				return err
			}
			checks, err := e.Reconcile(cmd.Context(), snap, base, branch)
			if err != nil {
				return err
			}
			if err := emit(cmd.OutOrStdout(), map[string]any{"base": base, "branches": checks}); err != nil {
				return err
			}
			bad := 0
			for _, c := range checks {
				if c.Stale || (c.Result != nil && !c.Result.OK) {
					bad++
				}
			}
			if bad > 0 {
				return output.New(output.CodeConflict, "%d branch(es) stale or outside their claim", bad)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&base, "base", "origin/main", "integration branch to diff against")
	cmd.Flags().StringVar(&branch, "branch", "", "only this branch")
	return cmd
}
