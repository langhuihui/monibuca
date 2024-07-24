package plugin_gb28181

import (
	"m7s.live/m7s/v5"
	"m7s.live/m7s/v5/pkg/util"
	gb28181 "m7s.live/m7s/v5/plugin/gb28181/pkg"
	"net/http"
	"os"
	"strings"
	"time"
)

func (gb *GB28181Plugin) replayPS(pub *m7s.Publisher, f *os.File) {
	defer f.Close()
	var t uint16
	receiver := gb28181.NewReceiver(pub)
	go receiver.Demux()
	defer close(receiver.FeedChan)
	for l := make([]byte, 6); pub.State != m7s.PublisherStateDisposed; time.Sleep(time.Millisecond * time.Duration(t)) {
		_, err := f.Read(l)
		if err != nil {
			return
		}
		payloadLen := util.ReadBE[int](l[:4])
		payload := make([]byte, payloadLen)
		t = util.ReadBE[uint16](l[4:])
		_, err = f.Read(payload)
		if err != nil {
			return
		}
		err = receiver.Unmarshal(payload)
		if err != nil {
			return
		}
		receiver.FeedChan <- receiver.Payload
	}
}

func (gb *GB28181Plugin) api_ps_replay(w http.ResponseWriter, r *http.Request) {
	dump := r.URL.Query().Get("dump")
	streamPath := r.PathValue("streamPath")
	if dump == "" {
		dump = "dump/ps"
	}
	f, err := os.OpenFile(dump, os.O_RDONLY, 0644)
	if err != nil {
		util.ReturnError(http.StatusInternalServerError, err.Error(), w, r)
	} else {
		if streamPath == "" {
			if strings.HasPrefix(dump, "/") {
				streamPath = "replay" + dump
			} else {
				streamPath = "replay/" + dump
			}
		}
		var pub *m7s.Publisher
		if pub, err = gb.Publish(streamPath, f); err == nil {
			go gb.replayPS(pub, f)
			util.ReturnOK(w, r)
		} else {
			util.ReturnError(http.StatusInternalServerError, err.Error(), w, r)
		}
	}
}
