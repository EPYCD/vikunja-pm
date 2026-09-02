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

package notify

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"code.vikunja.io/veans/internal/marshal/discord"
)

func delivery(event string, data string) Delivery {
	return Delivery{EventName: event, Time: time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC), Data: json.RawMessage(data)}
}

func TestFromDelivery(t *testing.T) {
	f := Formatter{TaskURL: func(id int64) string { return fmt.Sprintf("https://b/tasks/%d", id) }}
	task := `"task":{"id":12,"title":"A scheduler","identifier":"CY-12","done":%t,"leases":[{"pattern":"src/x.ts"}]}`
	doer := `"doer":{"username":"bot-alice","name":""}`

	cases := []struct {
		event  string
		data   string
		want   string
		colour int
		nilMsg bool
	}{
		{"task.claimed", `{` + fmt.Sprintf(task, false) + `,` + doer + `}`, "🔒 Claimed CY-12 · A scheduler", discord.ColourClaimed, false},
		{"task.updated", `{` + fmt.Sprintf(task, false) + `,` + doer + `}`, "", 0, true},
		{"task.updated", `{` + fmt.Sprintf(task, true) + `,` + doer + `}`, "✅ Done CY-12 · A scheduler", discord.ColourDone, false},
		{"task.leases.released", `{` + fmt.Sprintf(task, false) + `}`, "🔓 Files released CY-12", discord.ColourReleased, false},
		{"task.comment.created", `{` + fmt.Sprintf(task, false) + `,"comment":{"comment":"<p>Opened <a href=\"https://github.com/o/r/pull/7\">PR #7</a></p>"}}`, "🔗 Pull request on CY-12", discord.ColourReview, false},
		{"task.comment.created", `{` + fmt.Sprintf(task, false) + `,"comment":{"comment":"<p>plain</p>"}}`, "", 0, true},
		{"task.receipt.created", `{` + fmt.Sprintf(task, false) + `,"receipt":{"commit_sha":"abcdef1234567890","branch":"e5.3","passed":false,"merged":false,"ci_run_url":"https://ci/1","gates":[{"name":"test","status":"failed","duration_ms":30400}],"docs_api_required":true,"docs_api_regenerated":false}}`, "🧾 Gates red on CY-12", discord.ColourFailed, false},
		{"project.updated", `{"project":{"title":"x"}}`, "", 0, true},
	}
	for _, c := range cases {
		m := f.FromDelivery(delivery(c.event, c.data))
		if c.nilMsg {
			if m != nil {
				t.Errorf("%s: expected no card, got %+v", c.event, m)
			}
			continue
		}
		if m == nil || len(m.Embeds) != 1 {
			t.Fatalf("%s: expected one embed, got %+v", c.event, m)
		}
		e := m.Embeds[0]
		if !strings.HasPrefix(e.Title, c.want) {
			t.Errorf("%s: title %q, want prefix %q", c.event, e.Title, c.want)
		}
		if e.Colour != c.colour {
			t.Errorf("%s: colour %x, want %x", c.event, e.Colour, c.colour)
		}
		if e.URL != "https://b/tasks/12" {
			t.Errorf("%s: url %q", c.event, e.URL)
		}
	}

	receipt := f.FromDelivery(delivery("task.receipt.created", `{`+fmt.Sprintf(task, false)+`,"receipt":{"commit_sha":"abcdef1234567890","branch":"e5.3","passed":true,"merged":true,"gates":[{"name":"typecheck","status":"passed","duration_ms":1200},{"name":"build","status":"skipped","duration_ms":0}],"docs_api_required":true,"docs_api_regenerated":true}}`))
	fields := map[string]string{}
	for _, fd := range receipt.Embeds[0].Fields {
		fields[fd.Name] = fd.Value
	}
	if !strings.Contains(fields["Gates"], "✅ typecheck 1.2s") || !strings.Contains(fields["Gates"], "⏭ build 0ms") {
		t.Errorf("gates field: %q", fields["Gates"])
	}
	if fields["docs:api"] != "required, regenerated" || fields["Merged"] != "yes" || !strings.HasPrefix(fields["Commit"], "abcdef123456 on `e5.3`") {
		t.Errorf("fields: %v", fields)
	}
}

func TestFromFinding(t *testing.T) {
	f := Formatter{TaskURL: func(id int64) string { return fmt.Sprintf("https://b/tasks/%d", id) }}
	m := f.FromFinding(Finding{Kind: "drift", TaskID: 3, Identifier: "CY-3", Title: "x", Summary: "FR-161 changed"})
	if m.Embeds[0].Title != "📐 Spec drift · CY-3 x" || m.Embeds[0].URL != "https://b/tasks/3" || m.Embeds[0].Colour != discord.ColourWarning {
		t.Fatalf("%+v", m.Embeds[0])
	}
	h := f.FromFinding(Finding{Kind: "health", Summary: "ok"})
	if h.Embeds[0].Title != "🩺 Board health" || h.Embeds[0].URL != "" {
		t.Fatalf("%+v", h.Embeds[0])
	}
}

func TestHumanMS(t *testing.T) {
	for ms, want := range map[int64]string{250: "250ms", 1200: "1.2s", 59999: "60.0s", 123000: "2m03s"} {
		if got := humanMS(ms); got != want {
			t.Errorf("%d: %q want %q", ms, got, want)
		}
	}
}
