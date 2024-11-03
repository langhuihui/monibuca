package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
	m7s "m7s.live/pro"
	_ "m7s.live/pro/plugin/console"
	_ "m7s.live/pro/plugin/debug"
	_ "m7s.live/pro/plugin/flv"
	_ "m7s.live/pro/plugin/gb28181"
	_ "m7s.live/pro/plugin/logrotate"
	_ "m7s.live/pro/plugin/monitor"
	_ "m7s.live/pro/plugin/mp4"
	mp4 "m7s.live/pro/plugin/mp4/pkg"
	_ "m7s.live/pro/plugin/preview"
	_ "m7s.live/pro/plugin/rtmp"
	_ "m7s.live/pro/plugin/rtsp"
	_ "m7s.live/pro/plugin/sei"
	_ "m7s.live/pro/plugin/srt"
	_ "m7s.live/pro/plugin/stress"
	_ "m7s.live/pro/plugin/transcode"
	_ "m7s.live/pro/plugin/webrtc"
)

func getValue(yamlData map[string]interface{}, keys ...string) interface{} {
	current := yamlData

	for _, key := range keys {
		if value, exists := current[key]; exists {
			switch v := value.(type) {
			case map[string]interface{}:
				current = v
			default:
				return v // 返回找到的值
			}
		} else {
			return nil // 如果节点不存在，返回 nil
		}
	}

	return nil // 如果没有找到，返回 nil
}

func main() {
	conf := flag.String("c", "config.yaml", "config file")
	flag.Parse()
	// job.StreamPath live/test/001
	// job.FilePath  record/live/test/001

	confData, err := os.ReadFile(*conf)
	if err != nil {
		panic(err)
	}
	var confMap map[string]any
	err = yaml.Unmarshal(confData, &confMap)
	if err != nil {
		panic(err)
	}
	delPart := -1
	delPartVal := getValue(confMap, "mp4", "delpart")
	if tmp, ok := delPartVal.(int); ok {
		delPart = tmp
	}
	println(delPart)

	mp4.CustomFileName = func(job *m7s.RecordJob) string {

		fileDir := strings.ReplaceAll(job.FilePath, job.StreamPath, "")
		if err := os.MkdirAll(fileDir, 0755); err != nil {
			log.Default().Printf("创建目录失败：%s", err)
			return fmt.Sprintf("%s_%s%s", job.StreamPath, time.Now().Local().Format("2006-01-02-15-04-05"), ".mp4")
		}

		var recordName string

		streamParts := strings.Split(job.StreamPath, "/")
		if delPart >= 0 && delPart < len(streamParts) {
			// 删除第i个元素
			streamParts = append(streamParts[:delPart], streamParts[delPart+1:]...)
		}

		recordName = strings.Join(streamParts, "_")
		recordName = fmt.Sprintf("%s_%s%s", recordName, time.Now().Local().Format("2006-01-02-15-04-05"), ".mp4")
		recordName = filepath.Join(fileDir, recordName)
		return recordName
	}
	// ctx, _ := context.WithDeadline(context.Background(), time.Now().Add(time.Second*100))
	m7s.Run(context.Background(), *conf)
}
