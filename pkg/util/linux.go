//go:build !windows
// +build !windows

package util

import (
	"fmt"
	"io/ioutil"
	"os"
)

func CreateShutdownScript() error {
	return ioutil.WriteFile("shutdown.sh", []byte(fmt.Sprintf("kill -9 %d", os.Getpid())), 0777)
}
