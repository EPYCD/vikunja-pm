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
	"strings"

	"github.com/spf13/cobra"

	"code.vikunja.io/veans/internal/client"
	"code.vikunja.io/veans/internal/output"
)

func newCheckCmd() *cobra.Command {
	var (
		base   string
		staged bool
		tasks  []string
	)
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Check the files you changed against your tasks' scopes before opening a PR",
		Long: `Lists the files changed since --base (default: the merge base with the
main branch, or the index with --staged), reads the task references from the
commit messages' Refs: trailers (or --task), and asks the server whether every
file is inside those tasks' paths_owned and outside every other task's lease.
With --staged and no trailer yet, the tasks the bot has claimed stand in.

Exit 0 when the change is clean. Exit non-zero with CONFLICT when a file is
leased by another task or, if your tasks declare paths_owned, when a file is
outside them — widen the scope (veans scope), split the work into another
task, or hand the file back. The PR check runs the same query.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			rt, err := loadRuntime()
			if err != nil {
				return err
			}
			files, err := changedFiles(cmd.Context(), base, staged)
			if err != nil {
				return err
			}
			if len(files) == 0 {
				return output.New(output.CodeValidation, "no changed files to check")
			}
			refs := tasks
			if len(refs) == 0 {
				refs, err = refsFromCommits(cmd.Context(), base)
				if err != nil {
					return err
				}
			}
			ids := make([]int64, 0, len(refs))
			for _, ref := range refs {
				id, err := rt.resolveTaskID(cmd.Context(), ref)
				if err != nil {
					return err
				}
				ids = append(ids, id)
			}
			if len(ids) == 0 {
				if !staged {
					return output.New(output.CodeValidation, "no Refs: trailers in the commits to check — pass --task")
				}
				// A pre-commit hook runs before the first commit carries a
				// trailer; what the bot has claimed is the truest statement
				// of what it is working on.
				ids, err = claimedTaskIDs(cmd.Context(), rt)
				if err != nil {
					return err
				}
			}
			res, err := rt.client.CheckScope(cmd.Context(), rt.cfg.ProjectID, &client.ScopeCheckRequest{
				TaskIDs:    ids,
				Files:      files,
				Repository: rt.cfg.Repository,
			})
			if err != nil {
				return err
			}
			if err := json.NewEncoder(cmd.OutOrStdout()).Encode(res); err != nil {
				return err
			}
			if !res.OK {
				return output.New(output.CodeConflict, "%d file(s) outside your tasks' scope, %d leased by another task — see stdout for the verdicts", res.Strays, res.Collisions)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&base, "base", "", "git ref to diff against (default: merge base with main/master)")
	cmd.Flags().BoolVar(&staged, "staged", false, "check the index instead of commits (pre-commit use)")
	cmd.Flags().StringSliceVar(&tasks, "task", nil, "task references to check against instead of the commits' Refs: trailers (repeatable)")
	return cmd
}

// changedFiles lists paths changed relative to base, or the staged paths.
func changedFiles(ctx context.Context, base string, staged bool) ([]string, error) {
	var out string
	var err error
	if staged {
		out, err = runGit(ctx, "diff", "--cached", "--name-only", "--diff-filter=ACMRD")
	} else {
		b, berr := resolveBase(ctx, base)
		if berr != nil {
			return nil, berr
		}
		out, err = runGit(ctx, "diff", "--name-only", "--diff-filter=ACMRD", b+"...HEAD")
	}
	if err != nil {
		return nil, output.Wrap(output.CodeUnknown, err, "git diff: %v", err)
	}
	return splitLines(out), nil
}

// resolveBase picks the branch a PR would target when none is given.
func resolveBase(ctx context.Context, base string) (string, error) {
	if base != "" {
		return base, nil
	}
	for _, candidate := range []string{"origin/main", "origin/master", "main", "master"} {
		if _, err := runGit(ctx, "rev-parse", "--verify", "--quiet", candidate); err == nil {
			return candidate, nil
		}
	}
	return "", output.New(output.CodeValidation, "cannot find a main branch to diff against — pass --base")
}

// refsFromCommits collects Refs: trailers from the commits since base, the
// same way the merge hook does.
func refsFromCommits(ctx context.Context, base string) ([]string, error) {
	b, err := resolveBase(ctx, base)
	if err != nil {
		return nil, err
	}
	body, err := runGit(ctx, "log", "--format=%B", b+"..HEAD")
	if err != nil {
		return nil, output.Wrap(output.CodeUnknown, err, "git log: %v", err)
	}
	seen := map[string]bool{}
	refs := []string{}
	for _, line := range splitLines(body) {
		lower := strings.ToLower(strings.TrimSpace(line))
		if !strings.HasPrefix(lower, "refs:") && !strings.HasPrefix(lower, "ref:") {
			continue
		}
		rest := strings.TrimSpace(line[strings.Index(line, ":")+1:])
		for _, tok := range strings.FieldsFunc(rest, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' }) {
			if tok != "" && !seen[tok] {
				seen[tok] = true
				refs = append(refs, tok)
			}
		}
	}
	return refs, nil
}

// claimedTaskIDs lists the open tasks assigned to the bot.
func claimedTaskIDs(ctx context.Context, rt *runtime) ([]int64, error) {
	tasks, err := rt.client.ListProjectTasks(ctx, rt.cfg.ProjectID, &client.TaskListOptions{Filter: "done = false"})
	if err != nil {
		return nil, err
	}
	ids := []int64{}
	for _, t := range tasks {
		if !t.Done && taskAssignedTo(t, rt.cfg.Bot.UserID) {
			ids = append(ids, t.ID)
		}
	}
	return ids, nil
}

func splitLines(s string) []string {
	out := []string{}
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}
