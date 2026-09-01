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

package migration

import (
	"time"

	"src.techknowlogick.com/xormigrate"
	"xorm.io/xorm"
)

// Mirrors models.TaskPathLease.
type taskPathLeases20260901214315 struct {
	ID        int64     `xorm:"bigint autoincr not null unique pk"`
	TaskID    int64     `xorm:"bigint not null index"`
	ProjectID int64     `xorm:"bigint not null index"`
	UserID    int64     `xorm:"bigint not null"`
	Pattern   string    `xorm:"varchar(500) not null"`
	Created   time.Time `xorm:"created not null"`
}

func (taskPathLeases20260901214315) TableName() string {
	return "task_path_leases"
}

func init() {
	migrations = append(migrations, &xormigrate.Migration{
		ID:          "20260901214315",
		Description: "Add task path leases: the files an in-progress task holds exclusively",
		Migrate: func(tx *xorm.Engine) error {
			// Brand-new table, so a plain Sync is safe: there are no existing
			// indexes for it to drop.
			return tx.Sync2(taskPathLeases20260901214315{}) //nolint:forbidigo // brand-new table, nothing to drop
		},
		Rollback: func(tx *xorm.Engine) error {
			return tx.DropTables(taskPathLeases20260901214315{})
		},
	})
}
