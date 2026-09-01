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

// Mirrors models.TaskScope. The three list columns use xorm's json type,
// which the project already relies on for api_tokens.permissions and
// project_views.filter across all three databases.
type taskScopes20260901214314 struct {
	ID            int64     `xorm:"bigint autoincr not null unique pk"`
	TaskID        int64     `xorm:"bigint not null unique index"`
	PathsOwned    []string  `xorm:"json null"`
	PathsAffected []string  `xorm:"json null"`
	Endpoints     []string  `xorm:"json null"`
	Notes         string    `xorm:"longtext null"`
	Created       time.Time `xorm:"created not null"`
	Updated       time.Time `xorm:"updated not null"`
}

func (taskScopes20260901214314) TableName() string {
	return "task_scopes"
}

func init() {
	migrations = append(migrations, &xormigrate.Migration{
		ID:          "20260901214314",
		Description: "Add task scopes: the files and endpoints a task touches",
		Migrate: func(tx *xorm.Engine) error {
			// Brand-new table, so a plain Sync is safe: there are no existing
			// indexes for it to drop.
			return tx.Sync2(taskScopes20260901214314{}) //nolint:forbidigo // brand-new table, nothing to drop
		},
		Rollback: func(tx *xorm.Engine) error {
			return tx.DropTables(taskScopes20260901214314{})
		},
	})
}
