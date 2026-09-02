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

	"code.vikunja.io/veans/internal/bootstrap"
	"code.vikunja.io/veans/internal/output"
)

func newHooksCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hooks",
		Short: "Install or remove the git pre-commit scope check",
	}
	var force bool
	install := &cobra.Command{
		Use:   "install",
		Short: "Write .git/hooks/pre-commit so every commit runs 'veans check --staged'",
		Long: `Installs a pre-commit hook that refuses commits touching files outside
your tasks' paths_owned or files another in-progress task has leased — the
same check the PR runs, caught before the commit exists. An existing hook
veans did not write is left alone unless --force is given. Set
VEANS_SKIP_CHECK=1 to bypass the hook for one commit.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := repoRootFromGit(cmd)
			if err != nil {
				return err
			}
			path, action, err := bootstrap.InstallGitPreCommitHook(root, force)
			if err != nil {
				if errors.Is(err, bootstrap.ErrForeignHook) {
					return output.Wrap(output.CodeConflict, err, "%s: %v (pass --force to replace it)", path, err)
				}
				return output.Wrap(output.CodeUnknown, err, "install pre-commit hook: %v", err)
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]string{"path": path, "action": action})
		},
	}
	install.Flags().BoolVar(&force, "force", false, "replace a pre-commit hook veans did not write")
	uninstall := &cobra.Command{
		Use:   "uninstall",
		Short: "Remove the pre-commit hook veans installed",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			root, err := repoRootFromGit(cmd)
			if err != nil {
				return err
			}
			path, action, err := bootstrap.UninstallGitPreCommitHook(root)
			if err != nil {
				if errors.Is(err, bootstrap.ErrForeignHook) {
					return output.Wrap(output.CodeConflict, err, "%s: %v — not touching it", path, err)
				}
				return output.Wrap(output.CodeUnknown, err, "uninstall pre-commit hook: %v", err)
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]string{"path": path, "action": action})
		},
	}
	cmd.AddCommand(install, uninstall)
	return cmd
}

func repoRootFromGit(cmd *cobra.Command) (string, error) {
	root, err := runGit(cmd.Context(), "rev-parse", "--show-toplevel")
	if err != nil {
		return "", output.Wrap(output.CodeValidation, err, "not inside a git repository: %v", err)
	}
	return root, nil
}
