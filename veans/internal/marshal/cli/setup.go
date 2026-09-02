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
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"code.vikunja.io/veans/internal/client"
	veansconfig "code.vikunja.io/veans/internal/config"
	"code.vikunja.io/veans/internal/credentials"
	"code.vikunja.io/veans/internal/marshal/board"
	"code.vikunja.io/veans/internal/marshal/config"
	"code.vikunja.io/veans/internal/output"
)

// setupResult is what `marshal setup` prints. The CI token appears exactly
// once, here, for the repository secret.
type setupResult struct {
	MarshalBot      string   `json:"marshal_bot"`
	CIBot           string   `json:"ci_bot"`
	CIBotID         int64    `json:"ci_bot_id"`
	CIToken         string   `json:"ci_token,omitempty"`
	ReceiptBotSet   bool     `json:"receipt_bot_set"`
	ClaimBucketSet  bool     `json:"claim_bucket_set"`
	WebhookURL      string   `json:"webhook_url,omitempty"`
	WebhookEvents   []string `json:"webhook_events,omitempty"`
	WebhookExisting bool     `json:"webhook_existing,omitempty"`
	Next            []string `json:"next"`
}

// webhookEvents is what the Discord relay and the ledger consume.
var webhookEvents = []string{
	"task.created", "task.updated", "task.deleted", "task.claimed",
	"task.leases.released", "task.receipt.created", "task.comment.created",
}

func newSetupCmd() *cobra.Command {
	var (
		token       string
		rotate      bool
		skipWebhook bool
	)
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Create the Marshal and CI identities, wire the board guards, register the webhook",
		Long: `Runs once per project as a project admin:

  1. creates bot-marshal-<repo> (the service identity) and bot-ci-<repo> (the
     receipt bot), shares the project with both and mints their tokens —
     Marshal's into the credential store, CI's printed once for the repository
     secret WORKMAN_TOKEN;
  2. sets the project's receipt_bot_id so "done" needs a merged, passing
     receipt and the reviewer rule applies;
  3. sets the kanban view's claim_bucket_id to the In Progress bucket so a
     drag on the board acquires the same lock as a claim;
  4. when serve.public_url is set, registers a signed webhook to
     <public_url>/webhooks/workman for the events the Discord relay reads.

--token is a project admin's API token or session token; it is used only for
this run and never stored.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if token == "" {
				token = os.Getenv("MARSHAL_SETUP_TOKEN")
			}
			if token == "" {
				return output.New(output.CodeAuth, "pass --token (a project admin's token) or set MARSHAL_SETUP_TOKEN")
			}
			mpath, err := config.Find("")
			if err != nil {
				return output.Wrap(output.CodeNotConfigured, err, "no .marshal.yml — run `marshal init` first")
			}
			mcfg, err := config.Load(mpath)
			if err != nil {
				return err
			}
			vpath, err := veansconfig.Find(mcfg.Dir())
			if err != nil {
				return output.Wrap(output.CodeNotConfigured, err, "no .veans.yml — run `veans init` first")
			}
			vcfg, err := veansconfig.Load(vpath)
			if err != nil {
				return err
			}
			return runSetup(cmd.Context(), cmd.OutOrStdout(), mcfg, vcfg, client.New(vcfg.Server, token), rotate, skipWebhook)
		},
	}
	cmd.Flags().StringVar(&token, "token", "", "a project admin's token (or MARSHAL_SETUP_TOKEN)")
	cmd.Flags().BoolVar(&rotate, "rotate", false, "mint fresh tokens even when the bots exist")
	cmd.Flags().BoolVar(&skipWebhook, "skip-webhook", false, "do not register the board webhook")
	return cmd
}

func runSetup(ctx context.Context, w interface{ Write([]byte) (int, error) }, mcfg *config.Config, vcfg *veansconfig.Config, admin *client.Client, rotate, skipWebhook bool) error {
	repo := strings.TrimPrefix(vcfg.Bot.Username, "bot-")
	res := &setupResult{MarshalBot: board.MarshalBotPrefix + repo, CIBot: board.CIBotPrefix + repo}
	store := credentials.Default()

	routes, err := admin.Routes(ctx)
	if err != nil {
		return err
	}

	marshalBot, marshalTok, err := ensureBot(ctx, admin, store, vcfg, res.MarshalBot, "Marshal", withWebhookGrants(client.PermissionsForBot(routes), routes), rotate)
	if err != nil {
		return err
	}
	_ = marshalBot
	if marshalTok != "" {
		if err := store.Set(vcfg.Server, res.MarshalBot, marshalTok); err != nil {
			return output.Wrap(output.CodeUnknown, err, "store Marshal token: %v", err)
		}
	}

	ciBot, ciTok, err := ensureBot(ctx, admin, store, vcfg, res.CIBot, "CI", client.PermissionsForCI(routes), rotate)
	if err != nil {
		return err
	}
	res.CIBotID = ciBot.ID
	res.CIToken = ciTok

	if _, err := admin.PatchProject(ctx, vcfg.ProjectID, map[string]any{"receipt_bot_id": ciBot.ID}); err != nil {
		return output.Wrap(output.CodeUnknown, err, "set receipt bot: %v", err)
	}
	res.ReceiptBotSet = true

	if vcfg.Buckets.InProgress != 0 {
		if _, err := admin.PatchProjectView(ctx, vcfg.ProjectID, vcfg.ViewID, map[string]any{"claim_bucket_id": vcfg.Buckets.InProgress}); err != nil {
			return output.Wrap(output.CodeUnknown, err, "set claim bucket: %v", err)
		}
		res.ClaimBucketSet = true
	}

	if !skipWebhook && mcfg.Serve.PublicURL != "" {
		if err := registerWebhook(ctx, admin, mcfg, vcfg, res); err != nil {
			return err
		}
	}

	res.Next = []string{
		"add ci_token as the GitHub secret WORKMAN_TOKEN (it is not shown again; rerun with --rotate to mint a new one)",
		"run `marshal serve` on the always-on machine (or the docker-compose in deploy/)",
		"agents: `marshal mcp` in their MCP config, credentials via VEANS_TOKEN",
	}
	if res.WebhookURL == "" {
		res.Next = append(res.Next, "set serve.public_url and rerun setup to register the board webhook")
	}
	return emit(w, res)
}

func withWebhookGrants(perms map[string][]string, routes map[string]client.RouteGroup) map[string][]string {
	if avail, ok := routes["projects"]; ok {
		for _, want := range []string{"webhooks", "webhooks_get", "webhooks_post", "webhooks_put", "webhooks_delete"} {
			if _, has := avail[want]; has {
				perms["projects"] = append(perms["projects"], want)
			}
		}
	}
	return perms
}

// ensureBot finds or creates a bot owned by the admin, shares the project
// with it and mints a token when it has none stored (or rotate is set).
func ensureBot(ctx context.Context, admin *client.Client, store credentials.Store, vcfg *veansconfig.Config, username, name string, perms map[string][]string, rotate bool) (*client.BotUser, string, error) {
	bot, err := admin.FindMyBotByUsername(ctx, username)
	if err != nil && !isNotFound(err) {
		return nil, "", err
	}
	created := false
	if bot == nil {
		bot, err = admin.CreateBotUser(ctx, username, name)
		if err != nil {
			return nil, "", output.Wrap(output.CodeUnknown, err, "create %s: %v", username, err)
		}
		created = true
	}
	if _, err := admin.ShareProjectWithUser(ctx, vcfg.ProjectID, &client.ProjectUser{Username: username, Permission: 1}); err != nil && !isConflict(err) {
		return nil, "", output.Wrap(output.CodeUnknown, err, "share project with %s: %v", username, err)
	}
	if !created && !rotate {
		if tok, err := store.Get(vcfg.Server, username); err == nil && tok != "" {
			return bot, "", nil
		}
	}
	tok, err := admin.CreateToken(ctx, &client.APIToken{Title: "marshal " + name, Permissions: perms, ExpiresAt: client.FarFuture, OwnerID: bot.ID})
	if err != nil {
		return nil, "", output.Wrap(output.CodeUnknown, err, "mint token for %s: %v", username, err)
	}
	return bot, tok.Token, nil
}

func registerWebhook(ctx context.Context, admin *client.Client, mcfg *config.Config, vcfg *veansconfig.Config, res *setupResult) error {
	target := mcfg.Serve.PublicURL + "/webhooks/workman"
	secret := mcfg.Serve.WebhookSecret
	if secret == "" {
		buf := make([]byte, 32)
		if _, err := rand.Read(buf); err != nil {
			return err
		}
		secret = hex.EncodeToString(buf)
		if err := os.MkdirAll(mcfg.StateDir, 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(mcfg.WebhookSecretPath(), []byte(secret+"\n"), 0o600); err != nil {
			return err
		}
	}
	available, err := admin.WebhookEvents(ctx)
	if err != nil {
		return output.Wrap(output.CodeUnknown, err, "list webhook events: %v", err)
	}
	known := map[string]bool{}
	for _, ev := range available {
		known[ev] = true
	}
	events := []string{}
	for _, ev := range webhookEvents {
		if known[ev] {
			events = append(events, ev)
		}
	}
	existing, err := admin.ListProjectWebhooks(ctx, vcfg.ProjectID)
	if err != nil {
		return output.Wrap(output.CodeUnknown, err, "list webhooks: %v", err)
	}
	for _, wh := range existing {
		if wh.TargetURL == target {
			wh.Events = events
			wh.Secret = secret
			if _, err := admin.UpdateProjectWebhook(ctx, vcfg.ProjectID, wh); err != nil {
				return output.Wrap(output.CodeUnknown, err, "update webhook: %v", err)
			}
			res.WebhookURL, res.WebhookEvents, res.WebhookExisting = target, events, true
			return nil
		}
	}
	if _, err := admin.CreateProjectWebhook(ctx, vcfg.ProjectID, &client.Webhook{TargetURL: target, Events: events, Secret: secret}); err != nil {
		return output.Wrap(output.CodeUnknown, err, "create webhook: %v", err)
	}
	res.WebhookURL, res.WebhookEvents = target, events
	return nil
}

func isNotFound(err error) bool {
	var oe *output.Error
	return errors.As(err, &oe) && oe.Code == output.CodeNotFound
}

func isConflict(err error) bool {
	var oe *output.Error
	if errors.As(err, &oe) && oe.Code == output.CodeConflict {
		return true
	}
	return strings.Contains(fmt.Sprint(err), "already")
}
