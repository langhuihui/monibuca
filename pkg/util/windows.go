//go:build windows
// +build windows

package util

import (
	"fmt"
	"io/ioutil"
	"os"
)

func CreateShutdownScript() error {
	return ioutil.WriteFile("shutdown.bat", []byte(fmt.Sprintf("taskkill /pid %d  -t  -f", os.Getpid())), 0777)
}
