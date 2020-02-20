package pm

import (
	"bytes"
	"github.com/langhuihui/monibuca/monica/util"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"
)

type InstanceDesc struct {
	Name    string
	Path    string
	Plugins []string
	Config  string
}

func (p *InstanceDesc) Command(name string, args ...string) (cmd *exec.Cmd) {
	cmd = exec.Command(name, args...)
	cmd.Dir = p.Path
	return
}

func (p *InstanceDesc) CreateDir(sse *util.SSE, clearDir bool) (err error) {
	if clearDir {
		err = os.RemoveAll(p.Path)
	}
	if err = os.MkdirAll(p.Path, 0666); err != nil {
		return
	}
	sse.WriteEvent("step", []byte("2:目录创建成功！"))
	if err = ioutil.WriteFile(path.Join(p.Path, "config.toml"), []byte(p.Config), 0666); err != nil {
		return
	}
	var build bytes.Buffer
	build.WriteString(`package main
import(
"github.com/langhuihui/monibuca/monica"`)
	for _, plugin := range p.Plugins {
		build.WriteString("\n_ \"")
		build.WriteString(plugin)
		build.WriteString("\"")
	}
	build.WriteString("\n)\n")
	build.WriteString(`
func main(){
	monica.Run("config.toml")
	select{}
}
`)
	if err = ioutil.WriteFile(path.Join(p.Path, "main.go"), build.Bytes(), 0666); err != nil {
		return
	}
	sse.WriteEvent("step", []byte("3:文件创建成功！"))
	if err = sse.WriteExec(p.Command("go", "mod", "init", p.Name)); err != nil {
		return
	}
	sse.WriteEvent("step", []byte("4:go mod 初始化完成！"))
	if err = sse.WriteExec(p.Command("go", "build")); err != nil {
		return
	}
	sse.WriteEvent("step", []byte("5:go build 成功！"))
	binFile := strings.TrimSuffix(p.Path, "/")
	_, binFile = path.Split(binFile)
	if err = p.CreateRestartFile(binFile); err != nil {
		return
	}
	return sse.WriteExec(p.RestartCmd())
}
