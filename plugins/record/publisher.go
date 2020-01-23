package record

import (
	"errors"
	. "github.com/langhuihui/monibuca/monica"
	"github.com/langhuihui/monibuca/monica/avformat"
	"os"
	"syscall"
)

type FlvFile struct {
	InputStream
}

func PublishFlvFile(streamPath string) error {
	if file, err := os.Open(config.Path + streamPath + ".flv"); err == nil {
		stream := FlvFile{}
		stream.UseTimestamp = true
		if stream.Publish(streamPath, &stream) {
			file.Seek(int64(len(avformat.FLVHeader)), syscall.FILE_BEGIN)
			for {
				if tag, err := avformat.ReadFLVTag(file); err == nil {
					switch tag.Type {
					case avformat.FLV_TAG_TYPE_AUDIO:
						stream.PushAudio(tag)
					case avformat.FLV_TAG_TYPE_VIDEO:
						stream.PushVideo(tag)
					}
				} else {
					return err
				}
			}
		} else {
			return errors.New("Bad Name")
		}
	} else {
		return err
	}
}
