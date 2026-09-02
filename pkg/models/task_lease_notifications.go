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
	"strconv"
	"time"

	"code.vikunja.io/api/pkg/config"
	"code.vikunja.io/api/pkg/i18n"
	"code.vikunja.io/api/pkg/notifications"
	"code.vikunja.io/api/pkg/user"
	"code.vikunja.io/api/pkg/utils"
)

// LeaseStaleNotification tells the human behind a lease holder (the bot's
// owner, or the holder themself) that the holder went quiet while still
// locking files. Sent once per lease.
type LeaseStaleNotification struct {
	Doer     *user.User     `json:"doer"`
	Task     *Task          `json:"task"`
	Lease    *TaskPathLease `json:"lease"`
	Project  *Project       `json:"project"`
	Inactive time.Duration  `json:"-"`
}

func (n *LeaseStaleNotification) ToTitle(lang string) string {
	return i18n.T(lang, "notifications.task.lease_stale.subject", n.Doer.GetName(), n.Task.Title, n.Task.GetFullIdentifier())
}

func (n *LeaseStaleNotification) ToMail(lang string) *notifications.Mail {
	return notifications.NewMail().
		Subject(n.ToTitle(lang)).
		Line(i18n.T(lang, "notifications.task.lease_stale.message", notifications.EscapeMarkdown(n.Doer.GetName()), notifications.EscapeMarkdown(n.Task.Title), n.Task.GetFullIdentifier(), utils.HumanizeDuration(n.Inactive, lang))).
		Action(i18n.T(lang, "notifications.common.actions.open_task"), config.ServicePublicURL.GetString()+"tasks/"+strconv.FormatInt(n.Task.ID, 10)).
		Line(i18n.T(lang, "notifications.common.have_nice_day"))
}

func (n *LeaseStaleNotification) ToDB() any {
	return n
}

func (n *LeaseStaleNotification) Name() string {
	return "task.lease.stale"
}

// SubjectID is the lease, so one notification per lease however many runs
// see it stale.
func (n *LeaseStaleNotification) SubjectID() int64 {
	return n.Lease.ID
}

func (n *LeaseStaleNotification) ProjectID() int64 {
	return n.Task.ProjectID
}
