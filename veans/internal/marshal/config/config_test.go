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

package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoad_DefaultsAndEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, Filename)
	body := `references:
  - {prefix: FR-, file: docs/prd.md}
  - {prefix: AD-, file: docs/spine.md}
serve:
  public_url: https://marshal.example.com/
pool:
  ports: "3100-3102"
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MARSHAL_WEBHOOK_SECRET", "s3cret")
	t.Setenv("MARSHAL_DISCORD_WEBHOOK", "https://discord/hook")
	t.Setenv("MARSHAL_LISTEN", "0.0.0.0:9000")
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Dir() != dir || c.PasteMinWords != 12 || c.Codeowners != ".github/CODEOWNERS" || c.StaleAfter != 72*time.Hour {
		t.Fatalf("defaults: %+v", c)
	}
	if len(c.SpecFiles) != 2 || c.SpecFiles[0] != "docs/prd.md" {
		t.Fatalf("spec files should default to the reference files: %v", c.SpecFiles)
	}
	if c.Serve.PublicURL != "https://marshal.example.com" || c.Serve.WebhookSecret != "s3cret" || c.Serve.Listen != "0.0.0.0:9000" || c.Serve.Poll != time.Minute {
		t.Fatalf("serve: %+v", c.Serve)
	}
	if c.Discord.WebhookURL != "https://discord/hook" || c.Discord.Username != "Marshal" {
		t.Fatalf("discord: %+v", c.Discord)
	}
	if len(c.Pool.Ports) != 3 || c.Pool.Ports[2] != 3102 {
		t.Fatalf("ports: %v", c.Pool.Ports)
	}
	if c.StateDir != filepath.Join(dir, ".marshal") || c.LedgerPath() != filepath.Join(dir, ".marshal", "ledger.jsonl") {
		t.Fatalf("state: %s", c.StateDir)
	}

	sub := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	found, err := Find(sub)
	if err != nil || found != path {
		t.Fatalf("Find from a sub-directory: %q %v", found, err)
	}
	if _, err := Find(t.TempDir()); err == nil {
		t.Fatal("Find outside any config must fail")
	}
}

func TestLoad_SecretFileFallbackAndValidation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, Filename)
	if err := os.WriteFile(path, []byte("references:\n  - {prefix: FR-, file: docs/prd.md}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".marshal"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".marshal", "webhook-secret"), []byte("fromfile\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil || c.Serve.WebhookSecret != "fromfile" {
		t.Fatalf("secret file fallback: %v %q", err, c.Serve.WebhookSecret)
	}

	if err := os.WriteFile(path, []byte("references:\n  - {prefix: '', file: x}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("a reference without a prefix must be rejected")
	}
	if err := os.WriteFile(path, []byte("serve:\n  poll: 1s\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("a 1s poll must be rejected")
	}
}
