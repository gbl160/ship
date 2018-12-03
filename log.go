// Copyright 2018 xgfone <xgfone@126.com>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ship

import (
	"fmt"
	"io"
	"log"
)

// NewNoLevelLogger returns a new Logger, which has no level,
// that's, its level is always DEBUG.
func NewNoLevelLogger(w io.Writer, flag ...int) Logger {
	_flag := log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile
	if len(flag) > 0 {
		_flag = flag[0]
	}
	return loggerT{logger: log.New(w, "", _flag)}
}

type loggerT struct {
	logger *log.Logger
}

func (l loggerT) output(level, format string, args ...interface{}) {
	l.logger.Output(4, fmt.Sprintf(level+format, args...))
}

func (l loggerT) Debug(format string, args ...interface{}) {
	l.output("[DBUG] ", format, args...)
}

func (l loggerT) Info(format string, args ...interface{}) {
	l.output("[INFO] ", format, args...)
}

func (l loggerT) Warn(format string, args ...interface{}) {
	l.output("[WARN] ", format, args...)
}

func (l loggerT) Error(format string, args ...interface{}) {
	l.output("[EROR] ", format, args...)
}
