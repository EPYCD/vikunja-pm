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

package models

import (
	"xorm.io/xorm"
)

// Verdicts a changed file can get from a scope check.
const (
	ScopeVerdictOwned         = "owned"
	ScopeVerdictAffected      = "affected"
	ScopeVerdictUnscoped      = "unscoped"
	ScopeVerdictLeasedByOther = "leased_by_other"
)

// ScopeCheckRequest is the body of POST /projects/{id}/scope-check: the tasks
// a change claims to implement and the files it actually touched.
type ScopeCheckRequest struct {
	TaskIDs []int64  `json:"task_ids" maxItems:"100" doc:"The tasks the change implements — typically the Refs: trailers of the commits. Files are judged against the union of their scopes."`
	Files   []string `json:"files" required:"true" minItems:"1" maxItems:"5000" doc:"Repository-relative paths the change modifies, added or deleted, as git diff --name-only prints them."`
	// Repository namespaces bare files the way scope paths are namespaced
	// in a multi-repo project; empty for single-repo projects.
	Repository string `json:"repository,omitempty" maxLength:"100" doc:"Repository name to prefix the files with when the project's scope paths use repo: prefixes. Leave empty for single-repository projects."`
}

// ScopeCheckFile is one file's verdict.
type ScopeCheckFile struct {
	Path         string  `json:"path" doc:"The file as checked, with the repository prefix applied if one was given."`
	Verdict      string  `json:"verdict" doc:"owned: covered by paths_owned of a referenced task. affected: only covered by paths_affected. unscoped: no referenced task declares it. leased_by_other: an in-progress task that is not referenced holds a lease covering it."`
	TaskIDs      []int64 `json:"task_ids" doc:"The referenced tasks whose scope covers the file (owned or affected)."`
	HeldByTaskID int64   `json:"held_by_task_id,omitempty" doc:"For leased_by_other: the task holding the lease."`
}

// ScopeCheckResult is the answer: whether the change stays inside what its
// tasks declared, and every file that does not.
type ScopeCheckResult struct {
	Enforced   bool             `json:"enforced" doc:"True when at least one referenced task declares paths_owned; without a declared scope there is nothing to enforce and unscoped files are informational."`
	OK         bool             `json:"ok" doc:"True when no file is leased by an unreferenced task and, if enforced, no file is unscoped."`
	Strays     int              `json:"strays" doc:"Files outside every referenced task's paths_owned (only counted when enforced)."`
	Affected   int              `json:"affected" doc:"Files covered only by paths_affected — declared as read-only, yet changed."`
	Collisions int              `json:"collisions" doc:"Files covered by a lease held by a task that is not referenced."`
	Files      []ScopeCheckFile `json:"files" doc:"The verdict for every file, in the order given."`
}

// CheckScope judges a set of changed files against the scopes of the tasks
// a change references and against everyone else's leases. It is the answer
// a pre-commit hook, `veans check` and the PR check all read. The caller has
// checked read access on the project.
func CheckScope(s *xorm.Session, projectID int64, req *ScopeCheckRequest) (*ScopeCheckResult, error) {
	scopes, err := getTaskScopesForTasks(s, req.TaskIDs)
	if err != nil {
		return nil, err
	}
	// A referenced task from another project would let a change borrow a
	// scope it has no business with; only this project's tasks count.
	if len(req.TaskIDs) > 0 {
		own := []*Task{}
		if err := s.In("id", req.TaskIDs).Where("project_id = ?", projectID).Cols("id").Find(&own); err != nil {
			return nil, err
		}
		allowed := map[int64]bool{}
		for _, t := range own {
			allowed[t.ID] = true
		}
		for id := range scopes {
			if !allowed[id] {
				delete(scopes, id)
			}
		}
	}

	res := &ScopeCheckResult{Files: make([]ScopeCheckFile, 0, len(req.Files))}
	for _, sc := range scopes {
		if len(sc.PathsOwned) > 0 {
			res.Enforced = true
			break
		}
	}

	others := []*TaskPathLease{}
	q := s.Where("project_id = ?", projectID)
	if len(req.TaskIDs) > 0 {
		q = q.NotIn("task_id", req.TaskIDs)
	}
	if err := q.Find(&others); err != nil {
		return nil, err
	}

	for _, raw := range req.Files {
		file := raw
		if req.Repository != "" && !hasRepoPrefixSegment(file) {
			file = req.Repository + ":" + file
		}
		norm, err := NormalizeScopePath(file)
		if err != nil {
			// A path git printed cannot be invalid in any way that matters
			// here; keep it verbatim rather than failing the whole check.
			norm = file
		}
		f := ScopeCheckFile{Path: norm, TaskIDs: []int64{}, Verdict: ScopeVerdictUnscoped}
		for _, l := range others {
			if PathCoveredBy(l.Pattern, norm) {
				f.Verdict = ScopeVerdictLeasedByOther
				f.HeldByTaskID = l.TaskID
				res.Collisions++
				break
			}
		}
		if f.Verdict != ScopeVerdictLeasedByOther {
			affectedOnly := false
			for id, sc := range scopes {
				if coveredByAny(sc.PathsOwned, norm) {
					f.TaskIDs = append(f.TaskIDs, id)
					f.Verdict = ScopeVerdictOwned
				} else if coveredByAny(sc.PathsAffected, norm) {
					f.TaskIDs = append(f.TaskIDs, id)
					affectedOnly = true
				}
			}
			if f.Verdict != ScopeVerdictOwned && affectedOnly {
				f.Verdict = ScopeVerdictAffected
				res.Affected++
			}
			if f.Verdict == ScopeVerdictUnscoped && res.Enforced {
				res.Strays++
			}
		}
		res.Files = append(res.Files, f)
	}
	res.OK = res.Collisions == 0 && (!res.Enforced || res.Strays == 0)
	return res, nil
}

func coveredByAny(patterns []string, file string) bool {
	for _, p := range patterns {
		if PathCoveredBy(p, file) {
			return true
		}
	}
	return false
}

func hasRepoPrefixSegment(p string) bool {
	repo, _ := splitRepoPrefix(p)
	return repo != ""
}
