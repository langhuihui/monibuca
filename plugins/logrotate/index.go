package logrotate

import (
	"fmt"
	. "github.com/langhuihui/monibuca/monica"
	"log"
	"os"
	"path"
	"time"
)

var config = new(LogRotate)

type LogRotate struct {
	Path        string
	Size        int64
	Days        int
	file        *os.File
	currentSize int64
	createTime  time.Time
	hours       float64
	splitFunc   func() bool
}

func init() {
	InstallPlugin(&PluginConfig{
		Name:   "LogRotate",
		Type:   PLUGIN_HOOK,
		Config: config,
		Run:    run,
	})
}
func run() {
	if config.Size > 0 {
		config.splitFunc = config.splitBySize
	} else {
		if config.Days == 0 {
			config.Days = 1
		}
		config.hours = float64(config.Days) * 24
		config.splitFunc = config.splitByTime
	}
	config.createTime = time.Now()
	file, err := os.OpenFile(path.Join(config.Path, fmt.Sprintf("%s.log", config.createTime.Format("2006-01-02T15:04:05"))), os.O_TRUNC|os.O_WRONLY|os.O_CREATE, 0666)
	if err == nil {
		config.file = file
		stat, _ := file.Stat()
		config.currentSize = stat.Size()
		AddWriter(config)
	} else {
		log.Println(err)
	}
}
func (l *LogRotate) splitBySize() bool {
	return l.currentSize >= l.Size
}
func (l *LogRotate) splitByTime() bool {
	return time.Since(l.createTime).Hours() > l.hours
}
func (l *LogRotate) Write(data []byte) (n int, err error) {
	n, err = l.file.Write(data)
	l.currentSize += int64(n)
	if err == nil {
		if l.splitFunc() {
			l.createTime = time.Now()
			if file, err := os.OpenFile(path.Join(l.Path, fmt.Sprintf("%s.log", l.createTime.Format("2006-01-02T15:04:05"))), os.O_TRUNC|os.O_WRONLY|os.O_CREATE, 0666); err == nil {
				l.file = file
				l.currentSize = 0
			}
		}
	}
	return
}
