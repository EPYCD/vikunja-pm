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
	"sort"

	"code.vikunja.io/api/pkg/user"

	"xorm.io/xorm"
)

// ProjectAgent is one user doing work in a project — a bot with the human
// who owns it, or a human — with how much they currently hold. It is what
// the board's lanes and the leases panel label agents with.
type ProjectAgent struct {
	User      *user.User `json:"user" doc:"The assignee or lease holder."`
	Owner     *user.User `json:"owner,omitempty" doc:"When the user is a bot, the human who owns it."`
	OpenTasks int        `json:"open_tasks" doc:"Open tasks in the project assigned to this user."`
	Leases    int        `json:"leases" doc:"Path leases this user holds in the project."`
}

// GetProjectAgents lists everyone assigned to an open task or holding a lease
// in the project, bots first with their owners. The caller has checked read
// access on the project.
func GetProjectAgents(s *xorm.Session, projectID int64) ([]*ProjectAgent, error) {
	open := []*Task{}
	if err := s.Where("project_id = ? AND done = ?", projectID, false).Cols("id").Find(&open); err != nil {
		return nil, err
	}
	byUser := map[int64]*ProjectAgent{}
	get := func(id int64) *ProjectAgent {
		a, ok := byUser[id]
		if !ok {
			a = &ProjectAgent{}
			byUser[id] = a
		}
		return a
	}
	if len(open) > 0 {
		ids := make([]int64, 0, len(open))
		for _, t := range open {
			ids = append(ids, t.ID)
		}
		assignees := []*TaskAssginee{}
		if err := s.In("task_id", ids).Find(&assignees); err != nil {
			return nil, err
		}
		for _, a := range assignees {
			get(a.UserID).OpenTasks++
		}
	}
	leases := []*TaskPathLease{}
	if err := s.Where("project_id = ?", projectID).Find(&leases); err != nil {
		return nil, err
	}
	for _, l := range leases {
		get(l.UserID).Leases++
	}
	if len(byUser) == 0 {
		return []*ProjectAgent{}, nil
	}
	userIDs := make([]int64, 0, len(byUser))
	for id := range byUser {
		userIDs = append(userIDs, id)
	}
	users, err := user.GetUsersByIDs(s, userIDs)
	if err != nil {
		return nil, err
	}
	ownerIDs := []int64{}
	for id, a := range byUser {
		a.User = users[id]
		if a.User != nil && a.User.IsBot() {
			ownerIDs = append(ownerIDs, a.User.BotOwnerID)
		}
	}
	owners := map[int64]*user.User{}
	if len(ownerIDs) > 0 {
		if owners, err = user.GetUsersByIDs(s, ownerIDs); err != nil {
			return nil, err
		}
	}
	out := make([]*ProjectAgent, 0, len(byUser))
	for _, a := range byUser {
		if a.User == nil {
			continue
		}
		if a.User.IsBot() {
			a.Owner = owners[a.User.BotOwnerID]
		}
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool {
		bi, bj := out[i].User.IsBot(), out[j].User.IsBot()
		if bi != bj {
			return bi
		}
		return out[i].User.Username < out[j].User.Username
	})
	return out, nil
}
