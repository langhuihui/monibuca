// +build !windows

package pm

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"
)

func HomeDir() (string, error) {
	// First prefer the HOME environmental variable
	if home := os.Getenv("HOME"); home != "" {
		return home, nil
	}

	// If that fails, try the shell
	var stdout bytes.Buffer
	cmd := exec.Command("sh", "-c", "eval echo ~$USER")
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", err
	}

	result := strings.TrimSpace(stdout.String())
	if result == "" {
		return "", errors.New("blank output when reading home directory")
	}

	return result, nil
}
func (p *InstanceDesc) ShutDownCmd() *exec.Cmd {
	return p.Command("sh", "shutdown.sh")
}
func (p *InstanceDesc) RestartCmd() *exec.Cmd {
	return p.Command("sh", "restart.sh")
}
func (p *InstanceDesc) CreateRestartFile(binFile string) error {
	return ioutil.WriteFile(path.Join(p.Path, "restart.sh"), []byte(fmt.Sprintf(`./shutdown.sh
%s &`, binFile)), 0777)
}
