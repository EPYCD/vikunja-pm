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

// Package notify turns board webhook deliveries and Marshal's own findings
// into Discord embeds. One event, one card; the prewritten shapes are the
// integration.
package notify

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"code.vikunja.io/veans/internal/marshal/discord"
)

// Delivery is the board's webhook body.
type Delivery struct {
	EventName string          `json:"event_name"`
	Time      time.Time       `json:"time"`
	Data      json.RawMessage `json:"data"`
}

// Event is the subset of the payload every card needs.
type Event struct {
	Task *struct {
		ID         int64  `json:"id"`
		Title      string `json:"title"`
		Identifier string `json:"identifier"`
		Done       bool   `json:"done"`
		Assignees  []struct {
			Username string `json:"username"`
			Name     string `json:"name"`
		} `json:"assignees"`
		Leases []struct {
			Pattern string `json:"pattern"`
		} `json:"leases"`
	} `json:"task"`
	Doer *struct {
		Username string `json:"username"`
		Name     string `json:"name"`
	} `json:"doer"`
	Project *struct {
		Title string `json:"title"`
	} `json:"project"`
	Comment *struct {
		Comment string `json:"comment"`
	} `json:"comment"`
	Receipt *struct {
		CommitSHA string `json:"commit_sha"`
		Branch    string `json:"branch"`
		Passed    bool   `json:"passed"`
		Merged    bool   `json:"merged"`
		CIRunURL  string `json:"ci_run_url"`
		Gates     []struct {
			Name       string `json:"name"`
			Status     string `json:"status"`
			DurationMS int64  `json:"duration_ms"`
		} `json:"gates"`
		DocsAPIRequired    bool `json:"docs_api_required"`
		DocsAPIRegenerated bool `json:"docs_api_regenerated"`
	} `json:"receipt"`
}

// Formatter builds cards with board links.
type Formatter struct {
	// TaskURL returns the board page of a task.
	TaskURL func(taskID int64) string
}

var prLink = regexp.MustCompile(`https?://[^\s"<>]+/pull/\d+`)

// FromDelivery renders a board event, or nil when the event is not worth a
// message (most task.updated deliveries).
func (f Formatter) FromDelivery(d Delivery) *discord.Message {
	var ev Event
	if err := json.Unmarshal(d.Data, &ev); err != nil || ev.Task == nil {
		return nil
	}
	actor := ""
	if ev.Doer != nil {
		actor = name(ev.Doer.Name, ev.Doer.Username)
	}
	id := ev.Task.Identifier
	if id == "" {
		id = fmt.Sprintf("#%d", ev.Task.ID)
	}
	title := func(verb string) string { return fmt.Sprintf("%s %s · %s", verb, id, ev.Task.Title) }
	e := discord.Embed{URL: f.TaskURL(ev.Task.ID), Timestamp: d.Time.UTC().Format(time.RFC3339)}
	if actor != "" {
		e.Author = &discord.Author{Name: actor}
	}
	if ev.Project != nil {
		e.Footer = &discord.Footer{Text: ev.Project.Title}
	}

	switch d.EventName {
	case "task.claimed":
		e.Title = title("🔒 Claimed")
		e.Colour = discord.ColourClaimed
		if paths := leasePatterns(&ev); paths != "" {
			e.Fields = append(e.Fields, discord.Field{Name: "Holds", Value: paths})
		}
	case "task.leases.released":
		e.Title = title("🔓 Files released")
		e.Colour = discord.ColourReleased
	case "task.updated":
		if !ev.Task.Done {
			return nil
		}
		e.Title = title("✅ Done")
		e.Colour = discord.ColourDone
	case "task.created":
		e.Title = title("🆕 Created")
		e.Colour = discord.ColourInfo
	case "task.deleted":
		e.Title = title("🗑 Deleted")
		e.Colour = discord.ColourReleased
	case "task.receipt.created":
		if ev.Receipt == nil {
			return nil
		}
		r := ev.Receipt
		if r.Passed {
			e.Title = title("🧾 Gates green on")
			e.Colour = discord.ColourDone
		} else {
			e.Title = title("🧾 Gates red on")
			e.Colour = discord.ColourFailed
		}
		gates := make([]string, 0, len(r.Gates))
		for _, g := range r.Gates {
			mark := "✅"
			switch g.Status {
			case "failed":
				mark = "❌"
			case "skipped":
				mark = "⏭"
			}
			gates = append(gates, fmt.Sprintf("%s %s %s", mark, g.Name, humanMS(g.DurationMS)))
		}
		e.Fields = append(e.Fields, discord.Field{Name: "Gates", Value: strings.Join(gates, "\n")})
		docs := "not needed"
		if r.DocsAPIRequired {
			docs = "required, missing"
			if r.DocsAPIRegenerated {
				docs = "required, regenerated"
			}
		}
		e.Fields = append(e.Fields,
			discord.Field{Name: "Commit", Value: shortSHA(r.CommitSHA) + branchSuffix(r.Branch), Inline: true},
			discord.Field{Name: "docs:api", Value: docs, Inline: true},
			discord.Field{Name: "Merged", Value: yesNo(r.Merged), Inline: true},
		)
		if r.CIRunURL != "" {
			e.Description = "[CI run](" + r.CIRunURL + ")"
		}
	case "task.comment.created":
		if ev.Comment == nil {
			return nil
		}
		pr := prLink.FindString(ev.Comment.Comment)
		if pr == "" {
			return nil
		}
		e.Title = title("🔗 Pull request on")
		e.Colour = discord.ColourReview
		e.Description = pr
	default:
		return nil
	}
	return &discord.Message{Embeds: []discord.Embed{e}}
}

// Finding is one of Marshal's own observations.
type Finding struct {
	Kind       string // drift, broken_ref, paste, stale, stray, health, worktree, refusal
	TaskID     int64
	Identifier string
	Title      string
	Summary    string
	Details    []discord.Field
	URL        string
}

// FromFinding renders a Marshal finding.
func (f Formatter) FromFinding(fd Finding) *discord.Message {
	e := discord.Embed{Fields: fd.Details, Description: fd.Summary}
	if fd.URL != "" {
		e.URL = fd.URL
	} else if fd.TaskID != 0 {
		e.URL = f.TaskURL(fd.TaskID)
	}
	id := fd.Identifier
	if id == "" && fd.TaskID != 0 {
		id = fmt.Sprintf("#%d", fd.TaskID)
	}
	label := strings.TrimSpace(id + " " + fd.Title)
	switch fd.Kind {
	case "drift":
		e.Title = "📐 Spec drift · " + label
		e.Colour = discord.ColourWarning
	case "broken_ref":
		e.Title = "🔗💥 Broken reference · " + label
		e.Colour = discord.ColourFailed
	case "paste":
		e.Title = "📋 Spec text pasted · " + label
		e.Colour = discord.ColourWarning
	case "stale":
		e.Title = "🕰 Stale claim · " + label
		e.Colour = discord.ColourWarning
	case "stray":
		e.Title = "🚧 Out-of-scope change · " + label
		e.Colour = discord.ColourFailed
	case "health":
		e.Title = "🩺 Board health"
		e.Colour = discord.ColourInfo
	case "worktree":
		e.Title = "🌱 Worktree · " + label
		e.Colour = discord.ColourInfo
	case "refusal":
		e.Title = "⛔ Refused · " + label
		e.Colour = discord.ColourFailed
	default:
		e.Title = label
		e.Colour = discord.ColourInfo
	}
	return &discord.Message{Embeds: []discord.Embed{e}}
}

func leasePatterns(ev *Event) string {
	if len(ev.Task.Leases) == 0 {
		return ""
	}
	out := make([]string, 0, len(ev.Task.Leases))
	for _, l := range ev.Task.Leases {
		out = append(out, "`"+l.Pattern+"`")
	}
	return strings.Join(out, ", ")
}

func name(full, username string) string {
	if full != "" {
		return full
	}
	return username
}

func shortSHA(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

func branchSuffix(b string) string {
	if b == "" {
		return ""
	}
	return " on `" + b + "`"
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func humanMS(ms int64) string {
	d := time.Duration(ms) * time.Millisecond
	switch {
	case d < time.Second:
		return fmt.Sprintf("%dms", ms)
	case d < time.Minute:
		return fmt.Sprintf("%.1fs", d.Seconds())
	default:
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
}
