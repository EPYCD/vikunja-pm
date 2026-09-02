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
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	veansconfig "code.vikunja.io/veans/internal/config"
	"code.vikunja.io/veans/internal/marshal/config"
	"code.vikunja.io/veans/internal/marshal/refs"
	"code.vikunja.io/veans/internal/output"
)

func newInitCmd() *cobra.Command {
	var (
		force    bool
		appRoot  string
		prd      string
		spine    string
		epics    string
		public   string
		discord  string
		dbs      []string
		ports    string
		codeown  string
		specRefs []string
	)
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Write .marshal.yml next to .veans.yml and ignore Marshal's state directory",
		Long: `Writes the configuration Marshal reads: which files hold which reference
prefixes (FR- in the PRD, AD- in the architecture spine, NFR-/D- in the epics),
where CODEOWNERS is, the sub-directory the board's paths are relative to, the
worker pool, and how to reach the service and Discord. Nothing in it is a copy
of the spec; it only says where the spec lives. Edit the file afterwards —
every key is documented in veans/README.md.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			vpath, err := veansconfig.Find("")
			if err != nil {
				if errors.Is(err, veansconfig.ErrNotFound) {
					return output.Wrap(output.CodeNotConfigured, err, "no .veans.yml found — run `veans init` first; Marshal shares its board coordinates")
				}
				return err
			}
			root := filepath.Dir(vpath)
			target := filepath.Join(root, config.Filename)
			if _, err := os.Stat(target); err == nil && !force {
				return output.New(output.CodeConflict, "%s exists — pass --force to overwrite", target)
			}

			cfg := &config.Config{AppRoot: appRoot, Codeowners: codeown}
			for _, spec := range specRefs {
				prefix, file, ok := strings.Cut(spec, "=")
				if !ok {
					return output.New(output.CodeValidation, "--ref %q: expected PREFIX=path/to/file.md", spec)
				}
				cfg.References = append(cfg.References, refs.Source{Prefix: prefix, File: file})
			}
			if len(cfg.References) == 0 {
				if prd != "" {
					cfg.References = append(cfg.References, refs.Source{Prefix: "FR-", File: prd}, refs.Source{Prefix: "NFR-", File: prd})
				}
				if spine != "" {
					cfg.References = append(cfg.References, refs.Source{Prefix: "AD-", File: spine})
				}
				if epics != "" {
					cfg.References = append(cfg.References, refs.Source{Prefix: "D-", File: epics})
				}
			}
			cfg.Pool.Databases = dbs
			if ports != "" {
				cfg.Pool.Ports, err = parsePortRange(ports)
				if err != nil {
					return err
				}
			}
			cfg.Serve.PublicURL = public
			cfg.Discord.WebhookURL = discord
			if err := cfg.SaveAs(target); err != nil {
				return err
			}
			if err := ensureIgnored(root, ".marshal/"); err != nil {
				return err
			}
			return emit(cmd.OutOrStdout(), map[string]any{"path": target, "references": len(cfg.References), "next": "edit the file, then `marshal setup --token <admin token>`"})
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "overwrite an existing .marshal.yml")
	cmd.Flags().StringVar(&appRoot, "app-root", "", "sub-directory the board's paths are relative to (e.g. captain-yard-web)")
	cmd.Flags().StringVar(&prd, "prd", "", "PRD file holding FR-/NFR- anchors")
	cmd.Flags().StringVar(&spine, "spine", "", "architecture file holding AD- anchors")
	cmd.Flags().StringVar(&epics, "epics", "", "epics file holding D- anchors")
	cmd.Flags().StringArrayVar(&specRefs, "ref", nil, "PREFIX=file, repeatable; overrides --prd/--spine/--epics")
	cmd.Flags().StringVar(&codeown, "codeowners", ".github/CODEOWNERS", "CODEOWNERS path")
	cmd.Flags().StringArrayVar(&dbs, "database", nil, "per-worker database URI, repeatable")
	cmd.Flags().StringVar(&ports, "ports", "", "per-worker dev ports, e.g. 3100-3110")
	cmd.Flags().StringVar(&public, "public-url", "", "how the board and CI reach Marshal, e.g. https://marshal.example.com")
	cmd.Flags().StringVar(&discord, "discord-webhook", "", "Discord channel webhook URL (or set MARSHAL_DISCORD_WEBHOOK)")
	return cmd
}

func parsePortRange(s string) ([]int, error) {
	lo, hi, ok := strings.Cut(s, "-")
	var a, b int
	if _, err := fmtSscan(lo, &a); err != nil {
		return nil, output.New(output.CodeValidation, "bad port range %q", s)
	}
	if !ok {
		return []int{a}, nil
	}
	if _, err := fmtSscan(hi, &b); err != nil || b < a {
		return nil, output.New(output.CodeValidation, "bad port range %q", s)
	}
	out := []int{}
	for p := a; p <= b; p++ {
		out = append(out, p)
	}
	return out, nil
}

func ensureIgnored(root, entry string) error {
	path := filepath.Join(root, ".gitignore")
	b, err := os.ReadFile(path) //nolint:gosec // the repository's own .gitignore
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(line) == entry {
			return nil
		}
	}
	content := string(b)
	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += entry + "\n"
	return os.WriteFile(path, []byte(content), 0o644) //nolint:gosec // .gitignore is world-readable by convention
}
