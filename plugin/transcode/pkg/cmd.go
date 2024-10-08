package transcode

import (
	"m7s.live/m7s/v5/pkg/task"
	"os"
	"os/exec"
)

type CommandTask struct {
	task.Task
	logFileName string
	logFile     *os.File
	Cmd         *exec.Cmd
}

func (ct *CommandTask) Start() (err error) {
	ct.SetDescription("cmd", ct.Cmd.String())
	if ct.logFileName != "" {
		ct.SetDescription("log", ct.logFileName)
		ct.logFile, err = os.OpenFile(ct.logFileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			ct.Error("Could not create transcode log", "err", err)
			return err
		}
		// 将命令的标准输出和标准错误输出重定向到日志文件
		ct.Cmd.Stdout = ct.logFile
		ct.Cmd.Stderr = ct.logFile

	} else {
		// 将命令的标准输出和标准错误输出重定向到操作系统的标准输出和标准错误输出
		ct.Cmd.Stdout = os.Stdout
		ct.Cmd.Stderr = os.Stderr
	}
	ct.Info("start exec", "cmd", ct.Cmd.String())
	err = ct.Cmd.Start()
	ct.SetDescription("pid", ct.Cmd.Process.Pid)
	return
}

func (ct *CommandTask) Dispose() {
	err := ct.Cmd.Process.Kill()
	ct.Info("kill", "err", err)
	if ct.logFile != nil {
		_ = ct.logFile.Close()
	}
}
