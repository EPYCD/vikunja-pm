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

// Package config is Marshal's .marshal.yml: what the repository's spec files
// are, where the chokepoints come from, the per-worker resource pool, and
// how to reach the board and Discord. Board coordinates come from the
// sibling .veans.yml so the two never disagree.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"code.vikunja.io/veans/internal/marshal/pool"
	"code.vikunja.io/veans/internal/marshal/refs"
	"code.vikunja.io/veans/internal/marshal/worktree"
)

// Filename is the config Marshal looks for, walking up from the working
// directory like .veans.yml.
const Filename = ".marshal.yml"

// ErrNotFound is returned when no .marshal.yml exists above cwd.
var ErrNotFound = errors.New(".marshal.yml not found")

// Config is the on-disk shape.
type Config struct {
	// References resolve at read time from these files.
	References []refs.Source `yaml:"references"`
	// SpecFiles feed the paste detector; defaults to the reference files.
	SpecFiles []string `yaml:"spec_files,omitempty"`
	// PasteMinWords is the verbatim run length that counts as a paste.
	PasteMinWords int `yaml:"paste_min_words,omitempty"`

	// Codeowners is the path of the CODEOWNERS file; the chokepoint list is
	// read from it and never hand-maintained.
	Codeowners string `yaml:"codeowners,omitempty"`
	// AppRoot is the sub-directory the board's paths are relative to when the
	// app is not at the repository root (CapYard: captain-yard-web).
	AppRoot string `yaml:"app_root,omitempty"`
	// DocsAPIPaths are the files whose change makes docs:api mandatory; the
	// receipt derives docs_api_required from the diff against them.
	DocsAPIPaths []string `yaml:"docs_api_paths,omitempty"`
	// DocsAPIOutput is the generated file whose presence in the diff means
	// the docs were regenerated.
	DocsAPIOutput string `yaml:"docs_api_output,omitempty"`

	Pool     pool.Config     `yaml:"pool"`
	Worktree worktree.Naming `yaml:"worktree"`
	// StaleAfter is how long a branch may go without a commit before its
	// claim is flagged.
	StaleAfter time.Duration `yaml:"stale_after,omitempty"`

	Serve   Serve   `yaml:"serve"`
	Discord Discord `yaml:"discord"`

	// StateDir holds the ledger and the pool registry; default .marshal/
	// next to the config (gitignored by `marshal init`).
	StateDir string `yaml:"state_dir,omitempty"`

	path string
}

// Serve configures the HTTP service.
type Serve struct {
	// Listen is the bind address, default 127.0.0.1:8090.
	Listen string `yaml:"listen,omitempty"`
	// PublicURL is how the board and GitHub reach Marshal through the tunnel,
	// e.g. https://marshal.example.com. Webhooks are registered against it.
	PublicURL string `yaml:"public_url,omitempty"`
	// WebhookSecret signs the board's deliveries. Env MARSHAL_WEBHOOK_SECRET
	// overrides it so it need not be committed.
	WebhookSecret string `yaml:"webhook_secret,omitempty"`
	// AllowOrigins are the board origins allowed to call the JSON API from
	// the browser (the Workman frontend).
	AllowOrigins []string `yaml:"allow_origins,omitempty"`
	// Poll is how often the background loops (drift, stale, invariants) run.
	Poll time.Duration `yaml:"poll,omitempty"`
}

// Discord is the notification channel.
type Discord struct {
	// WebhookURL is a Discord channel webhook. Env MARSHAL_DISCORD_WEBHOOK
	// overrides it.
	WebhookURL string `yaml:"webhook_url,omitempty"`
	Username   string `yaml:"username,omitempty"`
	AvatarURL  string `yaml:"avatar_url,omitempty"`
	// Events limits what is posted; empty means everything Marshal knows.
	Events []string `yaml:"events,omitempty"`
}

// Path returns where the config was loaded from.
func (c *Config) Path() string { return c.path }

// Dir returns the directory holding the config, i.e. the repository root.
func (c *Config) Dir() string { return filepath.Dir(c.path) }

// Find walks up from start (cwd when empty) to the first .marshal.yml.
func Find(start string) (string, error) {
	if start == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		start = wd
	}
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(dir, Filename)
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ErrNotFound
		}
		dir = parent
	}
}

// Load reads and validates a config, applying defaults and env overrides.
func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path) //nolint:gosec // the path is the user's own config
	if err != nil {
		return nil, err
	}
	c := &Config{}
	if err := yaml.Unmarshal(b, c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	c.path, err = filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	c.applyDefaults()
	if v := os.Getenv("MARSHAL_WEBHOOK_SECRET"); v != "" {
		c.Serve.WebhookSecret = v
	}
	if c.Serve.WebhookSecret == "" {
		// `marshal setup` keeps the secret out of the committed config.
		if b, err := os.ReadFile(c.WebhookSecretPath()); err == nil { //nolint:gosec // Marshal's own state file
			c.Serve.WebhookSecret = strings.TrimSpace(string(b))
		}
	}
	if v := os.Getenv("MARSHAL_DISCORD_WEBHOOK"); v != "" {
		c.Discord.WebhookURL = v
	}
	if v := os.Getenv("MARSHAL_PUBLIC_URL"); v != "" {
		c.Serve.PublicURL = strings.TrimRight(v, "/")
	}
	if v := os.Getenv("MARSHAL_LISTEN"); v != "" {
		c.Serve.Listen = v
	}
	return c, c.validate()
}

func (c *Config) applyDefaults() {
	if c.PasteMinWords == 0 {
		c.PasteMinWords = 12
	}
	if len(c.SpecFiles) == 0 {
		seen := map[string]bool{}
		for _, s := range c.References {
			if !seen[s.File] {
				seen[s.File] = true
				c.SpecFiles = append(c.SpecFiles, s.File)
			}
		}
	}
	if c.Codeowners == "" {
		c.Codeowners = ".github/CODEOWNERS"
	}
	if c.Worktree.Branch == "" {
		c.Worktree.Branch = worktree.DefaultBranch
	}
	if c.Worktree.Dir == "" {
		c.Worktree.Dir = worktree.DefaultDir
	}
	if c.StaleAfter == 0 {
		c.StaleAfter = 3 * 24 * time.Hour
	}
	if c.Serve.Listen == "" {
		c.Serve.Listen = "127.0.0.1:8090"
	}
	if c.Serve.Poll == 0 {
		c.Serve.Poll = 60 * time.Second
	}
	if c.Discord.Username == "" {
		c.Discord.Username = "Marshal"
	}
	if c.StateDir == "" {
		c.StateDir = filepath.Join(c.Dir(), ".marshal")
	} else if !filepath.IsAbs(c.StateDir) {
		c.StateDir = filepath.Join(c.Dir(), c.StateDir)
	}
	c.Serve.PublicURL = strings.TrimRight(c.Serve.PublicURL, "/")
}

func (c *Config) validate() error {
	for i, s := range c.References {
		if s.Prefix == "" || s.File == "" {
			return fmt.Errorf("references[%d]: prefix and file are required", i)
		}
	}
	if c.Serve.Poll < 5*time.Second {
		return errors.New("serve.poll must be at least 5s")
	}
	return nil
}

// LedgerPath is the append-only ledger file.
func (c *Config) LedgerPath() string { return filepath.Join(c.StateDir, "ledger.jsonl") }

// PoolPath is the pool registry file.
func (c *Config) PoolPath() string { return filepath.Join(c.StateDir, "pool.json") }

// WebhookSecretPath is where setup stores the board's signing secret.
func (c *Config) WebhookSecretPath() string { return filepath.Join(c.StateDir, "webhook-secret") }

// SaveAs writes the config; used by `marshal init`.
func (c *Config) SaveAs(path string) error {
	b, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}
