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
	"context"
	"time"

	"code.vikunja.io/api/pkg/config"
	"code.vikunja.io/api/pkg/cron"
	"code.vikunja.io/api/pkg/db"
	"code.vikunja.io/api/pkg/events"
	"code.vikunja.io/api/pkg/i18n"
	"code.vikunja.io/api/pkg/log"
	"code.vikunja.io/api/pkg/notifications"
	"code.vikunja.io/api/pkg/user"
	"code.vikunja.io/api/pkg/utils"

	"xorm.io/builder"
	"xorm.io/xorm"
)

// RegisterLeaseStaleCron polices path leases: it tells a holder's owner when
// the holder goes quiet, and — only when service.leaseautoreleaseafter is
// set — releases leases nobody has touched for that long.
func RegisterLeaseStaleCron() {
	if err := cron.Schedule("*/5 * * * *", func() { policeStaleLeasesAt(time.Now()) }); err != nil {
		log.Fatalf("Could not register stale lease cron: %s", err)
	}
}

func policeStaleLeasesAt(now time.Time) {
	const logPrefix = "[Stale leases] "
	s := db.NewSession()
	defer s.Close()
	if err := notifyStaleLeases(s, now); err != nil {
		_ = s.Rollback()
		log.Errorf(logPrefix+"notify: %s", err)
		return
	}
	if err := autoReleaseLeases(s, now); err != nil {
		_ = s.Rollback()
		events.CleanupPending(s)
		log.Errorf(logPrefix+"auto-release: %s", err)
		return
	}
	if err := s.Commit(); err != nil {
		events.CleanupPending(s)
		log.Errorf(logPrefix+"commit: %s", err)
		return
	}
	events.DispatchPending(context.Background(), s)
}

// notifyStaleLeases sends one notification per lease that crossed the stale
// threshold, to the holder's owner (bots cannot be notified) or the holder.
func notifyStaleLeases(s *xorm.Session, now time.Time) error {
	after := config.ServiceLeaseStaleAfter.GetDuration()
	if after <= 0 {
		return nil
	}
	leases := []*TaskPathLease{}
	if err := s.Where(builder.Lt{"last_active": now.Add(-after)}).And(builder.IsNull{"stale_notified_at"}).Find(&leases); err != nil {
		return err
	}
	if len(leases) == 0 {
		return nil
	}
	if err := embedLeaseHolders(s, leases); err != nil {
		return err
	}
	for _, l := range leases {
		if l.Task == nil || l.User == nil {
			continue
		}
		recipient := l.User
		if l.User.IsBot() {
			if l.Owner == nil {
				continue
			}
			recipient = l.Owner
		}
		project, err := GetProjectSimpleByID(s, l.ProjectID)
		if err != nil {
			return err
		}
		l.Task.setIdentifier(project)
		n := &LeaseStaleNotification{Doer: l.User, Task: l.Task, Lease: l, Project: project, Inactive: now.Sub(l.LastActive)}
		if err := notifications.Notify(recipient, n, s); err != nil {
			return err
		}
		if _, err := s.ID(l.ID).Cols("stale_notified_at").Update(&TaskPathLease{StaleNotifiedAt: now}); err != nil {
			return err
		}
	}
	return nil
}

// autoReleaseLeases drops leases whose holder has been silent longer than
// service.leaseautoreleaseafter, leaving a comment on the task so the
// history says why the files became free.
func autoReleaseLeases(s *xorm.Session, now time.Time) error {
	after := config.ServiceLeaseAutoReleaseAfter.GetDuration()
	if after <= 0 {
		return nil
	}
	leases := []*TaskPathLease{}
	if err := s.Where(builder.Lt{"last_active": now.Add(-after)}).Find(&leases); err != nil {
		return err
	}
	byTask := map[int64][]*TaskPathLease{}
	for _, l := range leases {
		byTask[l.TaskID] = append(byTask[l.TaskID], l)
	}
	for taskID, held := range byTask {
		task, err := GetTaskByIDSimple(s, taskID)
		if err != nil {
			if IsErrTaskDoesNotExist(err) {
				_ = releasePathLeasesForTask(s, taskID)
				continue
			}
			return err
		}
		holder, err := user.GetUserByID(s, held[0].UserID)
		if err != nil && !user.IsErrUserDoesNotExist(err) {
			return err
		}
		if err := releasePathLeasesForTask(s, taskID); err != nil {
			return err
		}
		lang := "en"
		if holder != nil {
			if holder.Language != "" {
				lang = holder.Language
			}
			events.DispatchOnCommit(s, &TaskLeasesReleasedEvent{Task: &task, Doer: holder})
		}
		comment := &TaskComment{
			TaskID:  taskID,
			Comment: "<p>" + i18n.T(lang, "notifications.task.lease_released.comment", utils.HumanizeDuration(after, lang)) + "</p>",
		}
		author := holder
		if author == nil {
			author = &user.User{ID: task.CreatedByID}
		}
		if err := comment.Create(s, author); err != nil {
			return err
		}
	}
	return nil
}
