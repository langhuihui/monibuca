// +build windows

package pm

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
)

func HomeDir() (string, error) {
	drive := os.Getenv("HOMEDRIVE")
	path := os.Getenv("HOMEPATH")
	home := drive + path
	if drive == "" || path == "" {
		home = os.Getenv("USERPROFILE")
	}
	if home == "" {
		return "", errors.New("HOMEDRIVE, HOMEPATH, and USERPROFILE are blank")
	}

	return home, nil
}
func (p *InstanceDesc) ShutDownCmd() *exec.Cmd {
	return p.Command("cmd", "/C", "shutdown.bat")
}

func (p *InstanceDesc) RestartCmd() *exec.Cmd {
	return p.Command("cmd", "/C", "restart.bat")
}
func (p *InstanceDesc) CreateRestartFile(binFile string) error {
	return ioutil.WriteFile(path.Join(p.Path, "restart.bat"), []byte(fmt.Sprintf(`call shutdown.bat
start %s`, binFile)), 0777)
}
