package plugin_gb28181

import (
	"context"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"m7s.live/m7s/v5"
	"m7s.live/m7s/v5/pkg/util"
	"m7s.live/m7s/v5/plugin/gb28181/pb"
	gb28181 "m7s.live/m7s/v5/plugin/gb28181/pkg"
	"net/http"
	"os"
	"strings"
	"time"
)

func (gb *GB28181Plugin) List(context.Context, *emptypb.Empty) (ret *pb.ResponseList, err error) {
	ret = &pb.ResponseList{}
	for d := range gb.devices.Range {
		var channels []*pb.Channel
		for c := range d.channels.Range {
			channels = append(channels, &pb.Channel{
				DeviceID:     c.DeviceID,
				ParentID:     c.ParentID,
				Name:         c.Name,
				Manufacturer: c.Manufacturer,
				Model:        c.Model,
				Owner:        c.Owner,
				CivilCode:    c.CivilCode,
				Address:      c.Address,
				Port:         int32(c.Port),
				Parental:     int32(c.Parental),
				SafetyWay:    int32(c.SafetyWay),
				RegisterWay:  int32(c.RegisterWay),
				Secrecy:      int32(c.Secrecy),
				Status:       string(c.Status),
				Longitude:    c.Longitude,
				Latitude:     c.Latitude,
				GpsTime:      timestamppb.New(c.GpsTime),
			})
		}
		ret.Data = append(ret.Data, &pb.Device{
			Id:           d.ID,
			Name:         d.Name,
			Manufacturer: d.Manufacturer,
			Model:        d.Model,
			Owner:        d.Owner,
			Status:       string(d.Status),
			Longitude:    d.Longitude,
			Latitude:     d.Latitude,
			GpsTime:      timestamppb.New(d.GpsTime),
			RegisterTime: timestamppb.New(d.StartTime),
			UpdateTime:   timestamppb.New(d.UpdateTime),
			Channels:     channels,
		})
	}
	return
}

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
		err = receiver.ReadRTP(payload)
		select {
		case receiver.FeedChan <- receiver.Packet.Payload:
		case <-pub.Done():
			return
		}
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
		if pub, err = gb.Publish(gb.Context, streamPath); err == nil {
			go gb.replayPS(pub, f)
			util.ReturnOK(w, r)
		} else {
			util.ReturnError(http.StatusInternalServerError, err.Error(), w, r)
		}
	}
}
