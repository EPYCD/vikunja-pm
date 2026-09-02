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
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"code.vikunja.io/veans/internal/client"
	"code.vikunja.io/veans/internal/marshal/discord"
	"code.vikunja.io/veans/internal/marshal/ledger"
	"code.vikunja.io/veans/internal/marshal/mcptools"
	"code.vikunja.io/veans/internal/marshal/notify"
	"code.vikunja.io/veans/internal/marshal/serve"
	"code.vikunja.io/veans/internal/output"
)

func newServeCmd() *cobra.Command {
	var (
		base    string
		specRev string
		once    bool
	)
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the service: webhook receiver, JSON API for the board panels, and the watcher",
		Long: `Listens on serve.listen (default 127.0.0.1:8090), receives the board's
webhooks at /webhooks/workman (HMAC-verified), answers the frontend panels
under /api/*, and every serve.poll fetches the spec revision, detects drift,
re-checks references, pastes, stale branches, strays and the invariants.
--once runs a single pass and prints it, for cron.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := load()
			if err != nil {
				return err
			}
			e.SpecRev = specRev
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			if once {
				return emit(cmd.OutOrStdout(), e.Tick(ctx, base))
			}
			s := serve.New(e, base, log.New(cmd.ErrOrStderr(), "", log.LstdFlags))
			return s.Run(ctx)
		},
	}
	cmd.Flags().StringVar(&base, "base", "origin/main", "integration branch branches are diffed against")
	cmd.Flags().StringVar(&specRev, "spec-rev", "origin/main", "revision the spec resolves at; empty reads the working tree")
	cmd.Flags().BoolVar(&once, "once", false, "run one pass and exit")
	return cmd
}

func newMCPCmd(version string) *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Serve the agent tools over MCP (stdio)",
		Long: `Speaks the Model Context Protocol on stdin/stdout. Every tool goes through
the same board API and the same refusals a human gets; the identity is the
token Marshal runs with (MARSHAL_TOKEN, or the agent's own veans bot token via
VEANS_TOKEN), so each agent shows up under its own name.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := load()
			if err != nil {
				return err
			}
			srv := mcptools.New(e, version)
			srv.Logger = log.New(cmd.ErrOrStderr(), "marshal-mcp ", log.LstdFlags)
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return srv.ServeStdio(ctx)
		},
	}
}

func newLedgerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ledger",
		Short: "The append-only record of acquires, releases, refusals and findings",
	}
	tail := &cobra.Command{
		Use:   "tail [n]",
		Short: "The last n entries (default 50)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := load()
			if err != nil {
				return err
			}
			entries, err := e.Ledger.Read()
			if err != nil {
				return err
			}
			n := 50
			if len(args) == 1 {
				if _, err := parseInt(args[0], &n); err != nil {
					return err
				}
			}
			if len(entries) > n {
				entries = entries[len(entries)-n:]
			}
			if entries == nil {
				entries = []ledger.Entry{}
			}
			return emit(cmd.OutOrStdout(), entries)
		},
	}
	verify := &cobra.Command{
		Use:   "verify",
		Short: "Walk the hash chain; non-zero when a line was edited or removed",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := load()
			if err != nil {
				return err
			}
			bad, err := e.Ledger.Verify()
			if err != nil {
				return err
			}
			if err := emit(cmd.OutOrStdout(), map[string]any{"path": e.Ledger.Path(), "intact": bad == 0, "broken_at": bad}); err != nil {
				return err
			}
			if bad != 0 {
				return output.New(output.CodeConflict, "ledger chain broken at seq %d", bad)
			}
			return nil
		},
	}
	cmd.AddCommand(tail, verify)
	return cmd
}

func parseInt(s string, out *int) (int, error) {
	var n int
	if _, err := fmtSscan(s, &n); err != nil || n <= 0 {
		return 0, output.New(output.CodeValidation, "expected a positive number, got %q", s)
	}
	*out = n
	return n, nil
}

func newNotifyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "notify",
		Short: "Discord: send a test card or replay a webhook payload",
	}
	test := &cobra.Command{
		Use:   "test",
		Short: "Post a test card to the configured Discord webhook",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			e, err := load()
			if err != nil {
				return err
			}
			if !e.Discord.Enabled() {
				return output.New(output.CodeNotConfigured, "no Discord webhook configured (discord.webhook_url or MARSHAL_DISCORD_WEBHOOK)")
			}
			msg := e.Format.FromFinding(notify.Finding{Kind: "health", Summary: "Marshal is connected to this channel.", Details: []discord.Field{{Name: "Board", Value: e.Board.Cfg.Server, Inline: true}, {Name: "Project", Value: e.Board.Cfg.ProjectIdentifier, Inline: true}}})
			if err := e.Discord.Send(cmd.Context(), *msg); err != nil {
				return err
			}
			return emit(cmd.OutOrStdout(), map[string]any{"sent": true, "at": time.Now().UTC()})
		},
	}
	replay := &cobra.Command{
		Use:   "replay <payload.json|->",
		Short: "Format a board webhook payload and post it (for testing the templates)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := load()
			if err != nil {
				return err
			}
			var raw []byte
			if args[0] == "-" {
				raw, err = readAll(cmd.InOrStdin())
			} else {
				raw, err = os.ReadFile(args[0])
			}
			if err != nil {
				return output.Wrap(output.CodeValidation, err, "read payload: %v", err)
			}
			var d notify.Delivery
			if err := json.Unmarshal(raw, &d); err != nil {
				return output.Wrap(output.CodeValidation, err, "parse payload: %v", err)
			}
			msg := e.Format.FromDelivery(d)
			if msg == nil {
				return emit(cmd.OutOrStdout(), map[string]any{"sent": false, "reason": "event " + d.EventName + " produces no card"})
			}
			if e.Discord.Enabled() {
				if err := e.Discord.Send(cmd.Context(), *msg); err != nil {
					return err
				}
			}
			return emit(cmd.OutOrStdout(), map[string]any{"sent": e.Discord.Enabled(), "message": msg})
		},
	}
	cmd.AddCommand(test, replay)
	return cmd
}

func newReceiptCmd() *cobra.Command {
	var (
		commit, branch, runURL, mergeSHA string
		gates                            []string
		merged, docsRequired, docsDone   bool
	)
	cmd := &cobra.Command{
		Use:   "receipt <task>...",
		Short: "Post a gate receipt as CI (needs the receipt bot's token)",
		Long: `Records a CI run on each task. --gate is repeatable as name=status[:duration_ms],
e.g. --gate typecheck=passed:1200 --gate test=failed:30400. The server
computes passed. GitHub Actions use the workman-merge-hook action instead;
this is for other CI systems and for repair.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			e, err := load()
			if err != nil {
				return err
			}
			parsed, err := parseGates(gates)
			if err != nil {
				return err
			}
			out := []*client.TaskReceipt{}
			for _, ref := range args {
				t, err := resolveTask(cmd.Context(), e, ref)
				if err != nil {
					return err
				}
				r, err := e.Board.Client.PostTaskReceipt(cmd.Context(), t.ID, &client.TaskReceipt{
					CommitSHA: commit, Branch: branch, Gates: parsed, CIRunURL: runURL,
					Merged: merged, MergeSHA: mergeSHA, DocsAPIRequired: docsRequired, DocsAPIRegenerated: docsDone,
				})
				if err != nil {
					return err
				}
				e.Log(ledger.Entry{Action: "receipt", TaskID: t.ID, Subject: commit, Outcome: map[bool]string{true: "passed", false: "failed"}[r.Passed], Metadata: map[string]any{"merged": r.Merged, "run": runURL}})
				out = append(out, r)
			}
			return emit(cmd.OutOrStdout(), out)
		},
	}
	cmd.Flags().StringVar(&commit, "commit", "", "commit sha the gates ran on (required)")
	cmd.Flags().StringVar(&branch, "branch", "", "branch name")
	cmd.Flags().StringVar(&runURL, "run-url", "", "link to the CI run")
	cmd.Flags().StringVar(&mergeSHA, "merge-sha", "", "merge commit, when merged")
	cmd.Flags().StringArrayVar(&gates, "gate", nil, "name=status[:duration_ms], repeatable")
	cmd.Flags().BoolVar(&merged, "merged", false, "the commit is on the merged branch")
	cmd.Flags().BoolVar(&docsRequired, "docs-api-required", false, "the diff touched the contract")
	cmd.Flags().BoolVar(&docsDone, "docs-api-regenerated", false, "the API docs were regenerated")
	_ = cmd.MarkFlagRequired("commit")
	_ = cmd.MarkFlagRequired("gate")
	return cmd
}

func parseGates(specs []string) ([]client.GateResult, error) {
	out := []client.GateResult{}
	for _, s := range specs {
		name, rest, ok := strings.Cut(s, "=")
		if !ok || name == "" {
			return nil, output.New(output.CodeValidation, "gate %q: expected name=status[:duration_ms]", s)
		}
		status, dur, _ := strings.Cut(rest, ":")
		g := client.GateResult{Name: name, Status: status}
		if dur != "" {
			var n int
			if _, err := fmtSscan(dur, &n); err != nil || n < 0 {
				return nil, output.New(output.CodeValidation, "gate %q: bad duration %q", s, dur)
			}
			g.DurationMS = int64(n)
		}
		out = append(out, g)
	}
	return out, nil
}
