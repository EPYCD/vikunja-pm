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

package ledger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLedger_ChainsAndVerifies(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ledger", "marshal.jsonl")
	l, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	first, err := l.Append(Entry{Action: "acquire", Actor: "bot-a", TaskID: 1, Subject: "src/a.ts", Outcome: "ok"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := l.Append(Entry{Action: "refuse", Actor: "bot-b", TaskID: 2, Subject: "src/a.ts", Outcome: "refused", Reason: "held by 1"})
	if err != nil {
		t.Fatal(err)
	}
	if first.Seq != 1 || second.Seq != 2 {
		t.Fatalf("seq: %d %d", first.Seq, second.Seq)
	}
	if second.Prev != first.Hash || first.Prev != "" {
		t.Fatalf("chain broken: %+v %+v", first, second)
	}
	entries, err := l.Read()
	if err != nil || len(entries) != 2 {
		t.Fatalf("read: %v %d", err, len(entries))
	}
	if bad, err := l.Verify(); err != nil || bad != 0 {
		t.Fatalf("verify: %v %d", err, bad)
	}

	// Editing the first line breaks the chain at line 1.
	raw, _ := os.ReadFile(path) //nolint:gosec // the test's own temp file
	tampered := strings.Replace(string(raw), `"outcome":"ok"`, `"outcome":"refused"`, 1)
	if err := os.WriteFile(path, []byte(tampered), 0o600); err != nil { //nolint:gosec // the test's own temp file
		t.Fatal(err)
	}
	bad, err := l.Verify()
	if err != nil || bad != 1 {
		t.Fatalf("expected tamper at seq 1: %v %d", err, bad)
	}
}

func TestLedger_EmptyIsIntact(t *testing.T) {
	l, err := Open(filepath.Join(t.TempDir(), "x.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if bad, err := l.Verify(); err != nil || bad != 0 {
		t.Fatalf("empty: %v %d", err, bad)
	}
}
