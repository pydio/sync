/*
 * Copyright 2019 Abstrium SAS
 *
 *  This file is part of Cells Sync.
 *
 *  Cells Sync is free software: you can redistribute it and/or modify
 *  it under the terms of the GNU General Public License as published by
 *  the Free Software Foundation, either version 3 of the License, or
 *  (at your option) any later version.
 *
 *  Cells Sync is distributed in the hope that it will be useful,
 *  but WITHOUT ANY WARRANTY; without even the implied warranty of
 *  MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 *  GNU General Public License for more details.
 *
 *  You should have received a copy of the GNU General Public License
 *  along with Cells Sync.  If not, see <https://www.gnu.org/licenses/>.
 */

package cmd

import (
	"os"
	"path/filepath"

	"github.com/kardianos/service"
	"github.com/spf13/cobra"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/pydio/cells-sync/config"
	"github.com/pydio/cells-sync/control"
	"github.com/pydio/cells/common/log"
)

func runner() {
	logs := config.Default().Logs
	os.MkdirAll(logs.Folder, 0755)
	log.RegisterWriteSyncer(zapcore.AddSync(&lumberjack.Logger{
		Filename:   filepath.Join(logs.Folder, "sync.log"),
		MaxAge:     logs.MaxAgeDays,   // days
		MaxSize:    logs.MaxFilesSize, // megabytes
		MaxBackups: logs.MaxFilesNumber,
	}))
	s := control.NewSupervisor()
	s.Serve()
}

var svcConfig = &service.Config{
	Name:        "com.pydio.CellsSync",
	DisplayName: "Cells Sync",
	Description: "Synchronization tool for Pydio Cells",
	Arguments:   []string{"start"},
	Option: map[string]interface{}{
		"UserService": true,
		"RunAtLoad":   true,
	},
}

type program struct{}

// Start should not block. Do the actual work async.
func (p *program) Start(s service.Service) error {
	go runner()
	return nil
}

// Stop should not block. Return with a few seconds.
func (p *program) Stop(s service.Service) error {
	return nil
}

var StartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start sync tasks",
	Run: func(cmd *cobra.Command, args []string) {
		if service.Interactive() {
			runner()
		} else {
			prg := &program{}
			s, err := service.New(prg, svcConfig)
			if err != nil {
				log.Fatal(err.Error())
				return
			}
			l, err := s.Logger(nil)
			if err != nil {
				log.Fatal(err.Error())
				return
			}
			err = s.Run()
			if err != nil {
				l.Error(err)
			}
		}
	},
}

func init() {
	RootCmd.AddCommand(StartCmd)
}
