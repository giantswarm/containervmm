/*

Copyright 2020 Salvatore Mazzarino

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		https://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package logs

import (
	golog "log"
	"os"

	"github.com/kata-containers/govmm/qemu"
	log "github.com/sirupsen/logrus"
)

// embed logrus logger to implement
// V method from QMPLog Interface
type logger struct {
	*log.Logger
}

var Logger *logger

var _ qemu.QMPLog = &logger{}

func newLogger() *logger {
	l := &logger{
		Logger: log.StandardLogger(),
	}

	return l
}

func init() {
	Logger = newLogger()

	Logger.SetOutput(os.Stdout)

	// Disable timestamp logging, but still output the seconds elapsed
	Logger.SetFormatter(&log.TextFormatter{
		DisableTimestamp: false,
		FullTimestamp:    true,
	})

	// Disable the stdlib's automatic add of the timestamp in beginning of the log message,
	// as we stream the logs from stdlib log to this logrus instance.
	golog.SetFlags(0)
	golog.SetOutput(Logger.Writer())
}

func (l logger) V(level int32) bool {
	return l.IsLevelEnabled(log.Level(level))
}
