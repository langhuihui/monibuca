package plugin_flv

import (
	"bufio"
	"encoding/binary"
	"io"
	"io/fs"
	"m7s.live/m7s/v5/pkg/util"
	flv "m7s.live/m7s/v5/plugin/flv/pkg"
	rtmp "m7s.live/m7s/v5/plugin/rtmp/pkg"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func (plugin *FLVPlugin) Download(w http.ResponseWriter, r *http.Request) {
	streamPath := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/download/"), ".flv")
	singleFile := filepath.Join(plugin.Path, streamPath+".flv")
	query := r.URL.Query()
	rangeStr := strings.Split(query.Get("range"), "~")
	var startTime, endTime time.Time
	if len(rangeStr) != 2 {
		http.NotFound(w, r)
		return
	}
	var err error
	startTime, err = util.TimeQueryParse(rangeStr[0])
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	endTime, err = util.TimeQueryParse(rangeStr[1])
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	timeRange := endTime.Sub(startTime)
	plugin.Info("download", "stream", streamPath, "start", startTime, "end", endTime)
	dir := filepath.Join(plugin.Path, streamPath)
	if util.Exist(singleFile) {

	} else if util.Exist(dir) {
		var fileList []fs.FileInfo
		var found bool
		var startOffsetTime time.Duration
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
					fileList = []fs.FileInfo{info}
					startOffsetTime = startTime.Sub(modTime)
					//fmt.Println(path, modTime, startTime, found)
					return nil
				}
			}
			if modTime.After(endTime) {
				return fs.ErrInvalid
			}
			fileList = append(fileList, info)
			return nil
		})
		if !found {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "video/x-flv")
		w.Header().Set("Content-Disposition", "attachment")
		var writer io.Writer = w
		flvHead := make([]byte, 9+4)
		tagHead := make(util.Buffer, 11)
		var contentLength uint64

		var amf *rtmp.AMF
		var metaData rtmp.EcmaArray
		initMetaData := func(reader io.Reader, dataLen uint32) {
			data := make([]byte, dataLen+4)
			_, err = io.ReadFull(reader, data)
			amf = &rtmp.AMF{
				Buffer: util.Buffer(data[1+2+len("onMetaData") : len(data)-4]),
			}
			var obj any
			obj, err = amf.Unmarshal()
			metaData = obj.(rtmp.EcmaArray)
		}
		var filepositions []uint64
		var times []float64
		for pass := 0; pass < 2; pass++ {
			offsetTime := startOffsetTime
			var offsetTimestamp, lastTimestamp uint32
			var init, seqAudioWritten, seqVideoWritten bool
			if pass == 1 {
				metaData["keyframes"] = map[string]any{
					"filepositions": filepositions,
					"times":         times,
				}
				amf.Marshals("onMetaData", metaData)
				offsetDelta := amf.Len() + 15
				offset := offsetDelta + len(flvHead)
				contentLength += uint64(offset)
				metaData["duration"] = timeRange.Seconds()
				metaData["filesize"] = contentLength
				for i := range filepositions {
					filepositions[i] += uint64(offset)
				}
				metaData["keyframes"] = map[string]any{
					"filepositions": filepositions,
					"times":         times,
				}
				amf.Reset()
				amf.Marshals("onMetaData", metaData)
				plugin.Info("start download", "metaData", metaData)
				w.Header().Set("Content-Length", strconv.FormatInt(int64(contentLength), 10))
				w.WriteHeader(http.StatusOK)
			}
			if offsetTime == 0 {
				init = true
			} else {
				offsetTimestamp = -uint32(offsetTime.Milliseconds())
			}
			for i, info := range fileList {
				if r.Context().Err() != nil {
					return
				}
				filePath := filepath.Join(dir, info.Name())
				plugin.Debug("read", "file", filePath)
				file, err := os.Open(filePath)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				reader := bufio.NewReader(file)
				if i == 0 {
					_, err = io.ReadFull(reader, flvHead)
					if pass == 1 {
						// 第一次写入头
						_, err = writer.Write(flvHead)
						tagHead[0] = flv.FLV_TAG_TYPE_SCRIPT
						l := amf.Len()
						tagHead[1] = byte(l >> 16)
						tagHead[2] = byte(l >> 8)
						tagHead[3] = byte(l)
						flv.PutFlvTimestamp(tagHead, 0)
						writer.Write(tagHead)
						writer.Write(amf.Buffer)
						l += 11
						binary.BigEndian.PutUint32(tagHead[:4], uint32(l))
						writer.Write(tagHead[:4])
					}
				} else {
					// 后面的头跳过
					_, err = reader.Discard(13)
					if !init {
						offsetTime = 0
						offsetTimestamp = 0
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
					lastTimestamp = tmp.ReadUint24() | uint32(tmp.ReadByte())<<24
					//fmt.Println(lastTimestamp, tagHead)
					if init {
						if t == flv.FLV_TAG_TYPE_SCRIPT {
							if pass == 0 {
								initMetaData(reader, dataLen)
							} else {
								_, err = reader.Discard(int(dataLen) + 4)
							}
						} else {
							lastTimestamp += offsetTimestamp
							if lastTimestamp >= uint32(timeRange.Milliseconds()) {
								break
							}
							if pass == 0 {
								data := make([]byte, dataLen+4)
								_, err = io.ReadFull(reader, data)
								frameType := (data[0] >> 4) & 0b0111
								idr := frameType == 1 || frameType == 4
								if idr {
									filepositions = append(filepositions, contentLength)
									times = append(times, float64(lastTimestamp)/1000)
								}
								contentLength += uint64(11 + dataLen + 4)
							} else {
								//fmt.Println("write", lastTimestamp)
								flv.PutFlvTimestamp(tagHead, lastTimestamp)
								_, err = writer.Write(tagHead)
								_, err = io.CopyN(writer, reader, int64(dataLen+4))
							}
						}
						continue
					}

					switch t {
					case flv.FLV_TAG_TYPE_SCRIPT:
						if pass == 0 {
							initMetaData(reader, dataLen)
						} else {
							_, err = reader.Discard(int(dataLen) + 4)
						}
					case flv.FLV_TAG_TYPE_AUDIO:
						if !seqAudioWritten {
							if pass == 0 {
								contentLength += uint64(11 + dataLen + 4)
								_, err = reader.Discard(int(dataLen) + 4)
							} else {
								flv.PutFlvTimestamp(tagHead, 0)
								_, err = writer.Write(tagHead)
								_, err = io.CopyN(writer, reader, int64(dataLen+4))
							}
							seqAudioWritten = true
						} else {
							_, err = reader.Discard(int(dataLen) + 4)
						}
					case flv.FLV_TAG_TYPE_VIDEO:
						if !seqVideoWritten {
							if pass == 0 {
								contentLength += uint64(11 + dataLen + 4)
								_, err = reader.Discard(int(dataLen) + 4)
							} else {
								flv.PutFlvTimestamp(tagHead, 0)
								_, err = writer.Write(tagHead)
								_, err = io.CopyN(writer, reader, int64(dataLen+4))
							}
							seqVideoWritten = true
						} else {
							if lastTimestamp >= uint32(offsetTime.Milliseconds()) {
								data := make([]byte, dataLen+4)
								_, err = io.ReadFull(reader, data)
								frameType := (data[0] >> 4) & 0b0111
								idr := frameType == 1 || frameType == 4
								if idr {
									init = true
									plugin.Debug("init", "lastTimestamp", lastTimestamp)
									if pass == 0 {
										filepositions = append(filepositions, contentLength)
										times = append(times, float64(lastTimestamp)/1000)
										contentLength += uint64(11 + dataLen + 4)
									} else {
										flv.PutFlvTimestamp(tagHead, 0)
										_, err = writer.Write(tagHead)
										_, err = writer.Write(data)
									}
								}
							} else {
								_, err = reader.Discard(int(dataLen) + 4)
							}
						}
					}
				}
				offsetTimestamp = lastTimestamp
				err = file.Close()
			}
		}
		plugin.Info("end download")
	} else {
		http.NotFound(w, r)
		return
	}
}
