package hls

import (
	"os"
	"path/filepath"
)

type TsInFile struct {
	TsInMemory
	file   *os.File
	path   string
	closed bool
}

func (ts *TsInFile) Open(path string) (err error) {
	dir := filepath.Dir(path)
	if err = os.MkdirAll(dir, 0755); err != nil {
		return
	}
	ts.file, err = os.Create(path)
	if err != nil {
		return err
	}
	ts.path = path
	return
}

func (ts *TsInFile) Close() error {
	if ts.closed || ts.file == nil {
		return nil
	}
	ts.closed = true
	return ts.file.Close()
}
