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

package cmd

import (
	"fmt"
	"os"

	"code.vikunja.io/api/pkg/config"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "vikunja",
	Short: "Workman is the work console you actually own.",
	Long: `Workman
The work console you actually own.

Projects, tasks, time and teams — self-hosted, with a web app, a desktop app,
CalDAV sync and a full REST API.

Workman is built on Vikunja and is licensed under the AGPL-3.0-or-later.

Find out more at vikunja.io.`,
	PreRun: webCmd.PreRun,
	Run:    webCmd.Run,
}

var configFlag string

func init() {
	rootCmd.PersistentFlags().StringVar(&configFlag, "config", "", "Path to the config file to use. Bypasses the default search path.")

	// Not PersistentPreRun: subcommands define their own, which shadows root's.
	cobra.OnInitialize(func() {
		if configFlag != "" {
			config.SetConfigFile(configFlag)
		}
	})
}

// Execute starts the application
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
