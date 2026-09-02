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

// Package worktree turns a claimed story into the exact shell commands rule 1
// asks for (one worktree and one branch per story) and inspects the worktrees
// a repository already has.
package worktree

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
)

const (
	// DefaultBranch is the branch template used when Naming.Branch is empty.
	DefaultBranch = "{{.Story}}-{{.Slug}}"
	// DefaultDir is the worktree directory template used when Naming.Dir is empty.
	DefaultDir = "../{{.Repo}}-{{.Story}}"

	maxSlugLen = 32
)

// Naming holds the Go text/template strings that shape branch and directory
// names. Fields: .Story (lowercased story id, "e5.3"), .Slug (kebab-case of
// the title, max 32 chars), .TaskID, .Identifier ("CY-12") and .Repo (the
// repository directory basename).
type Naming struct {
	Branch string `yaml:"branch"`
	Dir    string `yaml:"dir"`
}

// Plan is what a worker must run to start on a story.
type Plan struct {
	Story, Slug, Branch, Dir string
	// Commands are the shell lines to run, in order, with all values substituted.
	Commands []string
	// EnvLines are the "KEY=value" lines the plan appends to .env.local.
	EnvLines []string
}

type templateData struct {
	Story, Slug, Repo, Identifier string
	TaskID                        int64
}

var storyRe = regexp.MustCompile(`(?i)^([a-z])(\d+(?:\.\d+)?)([a-z]?)`)

// StoryID extracts the story id from a task title or identifier:
// "E5.3 — A scheduler" -> "E5.3"; "e0.1" -> "E0.1"; "" when there is none.
func StoryID(s string) string {
	s = strings.TrimSpace(s)
	m := storyRe.FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	// The match must end at a word boundary so "E53foo" is not "E53f". RE2
	// has no lookahead, and a trailing \b would let the engine backtrack
	// to a shorter match ("E5" out of "E5.3abc"), so check the remainder.
	if rest := s[len(m[0]):]; rest != "" && isAlnum(rest[0]) {
		return ""
	}
	return strings.ToUpper(m[1]) + m[2] + strings.ToLower(m[3])
}

func isAlnum(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// stopwords are dropped from slugs so "A scheduler" becomes "scheduler".
var stopwords = map[string]bool{
	"a": true, "an": true, "the": true, "and": true, "or": true, "of": true,
	"to": true, "for": true, "in": true, "on": true, "at": true, "by": true,
	"with": true, "from": true, "as": true, "is": true, "are": true, "be": true,
}

// Slug returns the kebab-case form of title: ASCII lowercase letters and
// digits, stopwords removed, at most 32 characters, cut on a word boundary
// where possible.
func Slug(title string) string {
	words := splitWords(title)
	kept := make([]string, 0, len(words))
	for _, w := range words {
		if !stopwords[w] {
			kept = append(kept, w)
		}
	}
	if len(kept) == 0 {
		kept = words
	}
	slug := strings.Join(kept, "-")
	if len(slug) > maxSlugLen {
		cut := maxSlugLen
		if slug[cut] != '-' {
			if i := strings.LastIndexByte(slug[:cut], '-'); i > 0 {
				cut = i
			}
		}
		slug = slug[:cut]
	}
	return strings.Trim(slug, "-")
}

func splitWords(s string) []string {
	var (
		words []string
		cur   strings.Builder
	)
	flush := func() {
		if cur.Len() > 0 {
			words = append(words, cur.String())
			cur.Reset()
		}
	}
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			cur.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return words
}

// Build renders the plan for a story. repoRoot is the current checkout: it
// supplies .Repo and anchors Plan.Dir as an absolute path, while Commands
// keep the relative form the template produced. database and port may be
// zero, in which case no .env.local line is emitted for them.
func Build(n Naming, repoRoot, story, title, identifier string, taskID int64, database string, port int) (Plan, error) {
	id := StoryID(story)
	if id == "" {
		id = StoryID(title)
	}
	if id == "" {
		return Plan{}, fmt.Errorf("worktree: no story id in story %q or title %q", story, title)
	}
	root, err := filepath.Abs(repoRoot)
	if err != nil {
		return Plan{}, fmt.Errorf("resolve repo root %q: %w", repoRoot, err)
	}
	data := templateData{
		Story:      strings.ToLower(id),
		Slug:       Slug(stripStory(title, id)),
		Repo:       filepath.Base(root),
		Identifier: identifier,
		TaskID:     taskID,
	}
	branch, err := render("branch", n.Branch, DefaultBranch, data)
	if err != nil {
		return Plan{}, err
	}
	dir, err := render("dir", n.Dir, DefaultDir, data)
	if err != nil {
		return Plan{}, err
	}
	absDir := dir
	if !filepath.IsAbs(dir) {
		absDir = filepath.Join(root, dir)
	}

	p := Plan{Story: id, Slug: data.Slug, Branch: branch, Dir: absDir}
	p.Commands = []string{
		fmt.Sprintf("git worktree add %s -b %s", shWord(dir), shWord(branch)),
		"cd " + shWord(dir),
	}
	if database != "" {
		p.EnvLines = append(p.EnvLines, "MONGODB_URI="+database)
	}
	if port != 0 {
		p.EnvLines = append(p.EnvLines, fmt.Sprintf("PORT=%d", port))
	}
	if len(p.EnvLines) > 0 {
		quoted := make([]string, len(p.EnvLines))
		for i, line := range p.EnvLines {
			quoted[i] = shQuote(line)
		}
		p.Commands = append(p.Commands, `printf '%s\n' `+strings.Join(quoted, " ")+" >> .env.local")
	}
	return p, nil
}

// stripStory removes a leading story id (and the separator after it) from a
// title so the slug does not repeat what the branch already starts with.
func stripStory(title, id string) string {
	t := strings.TrimSpace(title)
	if StoryID(t) != id {
		return t
	}
	return strings.TrimLeft(t[len(id):], " \t-–—:.|")
}

func render(name, tmpl, fallback string, data templateData) (string, error) {
	if strings.TrimSpace(tmpl) == "" {
		tmpl = fallback
	}
	t, err := template.New(name).Option("missingkey=error").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("parse %s template %q: %w", name, tmpl, err)
	}
	var b strings.Builder
	if err := t.Execute(&b, data); err != nil {
		return "", fmt.Errorf("render %s template %q: %w", name, tmpl, err)
	}
	// An empty slug would otherwise leave "e5.3-" behind.
	out := strings.TrimRight(strings.TrimSpace(b.String()), "-")
	if out == "" {
		return "", errors.New("worktree: " + name + " template rendered empty")
	}
	return out, nil
}

// shWord leaves tokens made of shell-safe characters bare so the default plan
// reads like the brief, and single-quotes anything else.
func shWord(s string) string {
	safe := func(r rune) bool {
		return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			strings.ContainsRune("-_./:@%+,", r)
	}
	if s != "" && strings.IndexFunc(s, func(r rune) bool { return !safe(r) }) < 0 {
		return s
	}
	return shQuote(s)
}

func shQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
