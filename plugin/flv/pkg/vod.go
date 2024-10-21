package flv

import (
	"bufio"
	"context"
	"io"
	"io/fs"
	"m7s.live/v5/pkg/util"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Vod struct {
	io.Writer
	Dir             string
	lastTimestamp   uint32
	speed           float64
	singleFile      bool
	offsetTime      time.Duration
	offsetTimestamp uint32
	fileList        []fs.FileInfo
	startTime       time.Time
}

func (v *Vod) SetSpeed(speed float64) {
	v.speed = speed
}

func (v *Vod) speedControl() {
	targetTime := time.Duration(float64(time.Since(v.startTime)) * v.speed)
	sleepTime := time.Duration(v.lastTimestamp)*time.Millisecond - targetTime
	//fmt.Println("sleepTime", sleepTime, time.Since(start).Milliseconds(), lastTimestamp)
	if sleepTime > 0 {
		time.Sleep(sleepTime)
	}
}

func (v *Vod) Init(startTime time.Time, dir string) (err error) {
	v.Dir = dir
	v.startTime = time.Now()
	singleFile := dir + ".flv"
	if util.Exist(singleFile) {
		v.singleFile = true
	} else if util.Exist(dir) {
		var found bool
		err = filepath.Walk(dir, func(path string, info fs.FileInfo, err error) error {
			if info.IsDir() || !strings.HasSuffix(info.Name(), ".flv") {
				return nil
			}
			modTime := info.ModTime()
			//tmp, _ := strconv.Atoi(strings.TrimSuffix(info.Name(), ".flv"))
			//fileStartTime := time.Unix(tmp, 10)
			if !found {
				if modTime.After(startTime) {
					found = true
					//fmt.Println(path, modTime, startTime, found)
				} else {
					v.fileList = []fs.FileInfo{info}
					v.offsetTime = startTime.Sub(modTime)
					//fmt.Println(path, modTime, startTime, found)
					return nil
				}
			}
			v.fileList = append(v.fileList, info)
			return nil
		})
		if !found {
			return os.ErrNotExist
		}
	}
	return
}

func (v *Vod) Run(ctx context.Context) (err error) {
	flvHead := make([]byte, 9+4)
	tagHead := make(util.Buffer, 11)
	var file *os.File
	var init, seqAudioWritten, seqVideoWritten bool
	if v.offsetTime == 0 {
		init = true
	} else {
		v.offsetTimestamp = -uint32(v.offsetTime.Milliseconds())
	}
	for i, info := range v.fileList {
		if ctx.Err() != nil {
			return
		}
		filePath := filepath.Join(v.Dir, info.Name())
		file, err = os.Open(filePath)
		if err != nil {
			return
		}
		reader := bufio.NewReader(file)
		if i == 0 {
			// 第一次写入头
			_, err = io.ReadFull(reader, flvHead)
			_, err = v.Write(flvHead)
		} else {
			// 后面的头跳过
			_, err = reader.Discard(13)
			if !init {
				v.offsetTime = 0
				v.offsetTimestamp = 0
			}
		}
		for err == nil {
			_, err = io.ReadFull(reader, tagHead)
			if err != nil {
				break
			}
			tmp := tagHead
			t := tmp.ReadByte()
			dataLen := tmp.ReadUint24()
			v.lastTimestamp = tmp.ReadUint24() | uint32(tmp.ReadByte())<<24
			//fmt.Println(lastTimestamp, tagHead)
			if init {
				if t == FLV_TAG_TYPE_SCRIPT {
					_, err = reader.Discard(int(dataLen) + 4)
				} else {
					v.lastTimestamp += v.offsetTimestamp
					PutFlvTimestamp(tagHead, v.lastTimestamp)
					_, err = v.Write(tagHead)
					_, err = io.CopyN(v, reader, int64(dataLen+4))
					v.speedControl()
				}
				continue
			}
			switch t {
			case FLV_TAG_TYPE_SCRIPT:
				_, err = reader.Discard(int(dataLen) + 4)
			case FLV_TAG_TYPE_AUDIO:
				if !seqAudioWritten {
					PutFlvTimestamp(tagHead, 0)
					_, err = v.Write(tagHead)
					_, err = io.CopyN(v, reader, int64(dataLen+4))
					seqAudioWritten = true
				} else {
					_, err = reader.Discard(int(dataLen) + 4)
				}
			case FLV_TAG_TYPE_VIDEO:
				if !seqVideoWritten {
					PutFlvTimestamp(tagHead, 0)
					_, err = v.Write(tagHead)
					_, err = io.CopyN(v, reader, int64(dataLen+4))
					seqVideoWritten = true
				} else {
					if v.lastTimestamp >= uint32(v.offsetTime.Milliseconds()) {
						data := make([]byte, dataLen+4)
						_, err = io.ReadFull(reader, data)
						frameType := (data[0] >> 4) & 0b0111
						idr := frameType == 1 || frameType == 4
						if idr {
							init = true
							//fmt.Println("init", lastTimestamp)
							PutFlvTimestamp(tagHead, 0)
							_, err = v.Write(tagHead)
							_, err = v.Write(data)
						}
					} else {
						_, err = reader.Discard(int(dataLen) + 4)
					}
				}
			}
		}
		v.offsetTimestamp = v.lastTimestamp
		err = file.Close()
	}
	return
}
