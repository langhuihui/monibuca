package plugin_gb28181pro

import (
	"context"
	"net/http"
	"strings"

	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"m7s.live/v5/pkg/config"
	"m7s.live/v5/plugin/gb28181pro/pb"
	gb28181 "m7s.live/v5/plugin/gb28181pro/pkg"
)

func (gb *GB28181ProPlugin) List(context.Context, *emptypb.Empty) (ret *pb.ResponseList, err error) {
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

func (gb *GB28181ProPlugin) api_ps_replay(w http.ResponseWriter, r *http.Request) {
	dump := r.URL.Query().Get("dump")
	streamPath := r.PathValue("streamPath")
	if dump == "" {
		dump = "dump/ps"
	}
	if streamPath == "" {
		if strings.HasPrefix(dump, "/") {
			streamPath = "replay" + dump
		} else {
			streamPath = "replay/" + dump
		}
	}
	var puller gb28181.DumpPuller
	puller.GetPullJob().Init(&puller, &gb.Plugin, streamPath, config.Pull{
		URL: dump,
	}, nil)
}

// GetDevice 实现获取单个设备信息
func (gb *GB28181ProPlugin) GetDevice(ctx context.Context, req *pb.GetDeviceRequest) (*pb.DeviceResponse, error) {
	resp := &pb.DeviceResponse{}
	if d := gb.devices.Get(req.DeviceId); d != nil {
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
		resp.Data = &pb.Device{
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
		}
		resp.Code = 0
		resp.Message = "success"
	} else {
		resp.Code = 404
		resp.Message = "device not found"
	}
	return resp, nil
}

// GetDevices 实现分页查询设备列表
func (gb *GB28181ProPlugin) GetDevices(ctx context.Context, req *pb.GetDevicesRequest) (*pb.DevicesPageInfo, error) {
	resp := &pb.DevicesPageInfo{}
	var devices []*pb.Device
	total := 0
	for d := range gb.devices.Range {
		// TODO: 实现查询条件过滤
		if req.Query != "" && !strings.Contains(d.ID, req.Query) && !strings.Contains(d.Name, req.Query) {
			continue
		}
		if req.Status && string(d.Status) != "ON" {
			continue
		}
		total++
		// 分页处理
		if total > int(req.Page*req.Count) {
			continue
		}
		if total <= int((req.Page-1)*req.Count) {
			continue
		}
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
		devices = append(devices, &pb.Device{
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
	resp.Total = int32(total)
	resp.List = devices
	resp.Code = 0
	resp.Message = "success"
	return resp, nil
}

// GetChannels 实现分页查询通道
func (gb *GB28181ProPlugin) GetChannels(ctx context.Context, req *pb.GetChannelsRequest) (*pb.ChannelsPageInfo, error) {
	resp := &pb.ChannelsPageInfo{}
	if d := gb.devices.Get(req.DeviceId); d != nil {
		var channels []*pb.Channel
		total := 0
		for c := range d.channels.Range {
			// TODO: 实现查询条件过滤
			if req.Query != "" && !strings.Contains(c.DeviceID, req.Query) && !strings.Contains(c.Name, req.Query) {
				continue
			}
			if req.Online && string(c.Status) != "ON" {
				continue
			}
			if req.ChannelType && c.ParentID == "" {
				continue
			}
			total++
			// 分页处理
			if total > int(req.Page*req.Count) {
				continue
			}
			if total <= int((req.Page-1)*req.Count) {
				continue
			}
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
		resp.Total = int32(total)
		resp.List = channels
		resp.Code = 0
		resp.Message = "success"
	} else {
		resp.Code = 404
		resp.Message = "device not found"
	}
	return resp, nil
}
