package plugin_onvif

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"m7s.live/v5/pkg/util"
	"net/http"
	"strconv"

	"github.com/IOTechSystems/onvif/xsd/onvif"
	"github.com/jinzhu/copier"
)

type Response struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data"`
}

type DeviceAddReq struct {
	IP       string `json:"ip"`
	Port     string `json:"port"`
	User     string `json:"user"`
	Password string `json:"passwd"`
	Path     string `json:"path"`
	Channel  int    `json:"channel"`
}

type PtzMoveReq struct {
	IP    string  `json:"ip"`
	Mode  int     `json:"mode"`
	Pan   float64 `json:"pan"`
	Tilt  float64 `json:"tilt"`
	Zoom  float64 `json:"zoom"`
	Speed float64 `json:"speed"`
}

type PtzPresetReq struct {
	IP          string `json:"ip"`
	PresetToken string `json:"preset_token"`
	PresetName  string `json:"preset_name"`
}

type ImagingReq struct {
	IP              string  `json:"ip"`
	Brightness      float64 `json:"brightness"`
	ColorSaturation float64 `json:"color_saturation"`
	Contrast        float64 `json:"contrast"`
	Sharpness       float64 `json:"sharpness"`
	Force           bool    `json:"force"`
}

type DeviceParam struct {
	Ip      string `json:"ip"`
	Port    string `json:"port"`
	User    string `json:"user"`
	Passwd  string `json:"passwd"`
	Path    string `json:"path"`
	IFace   string `json:"iface"`
	Channel int    `json:"channel"`
}

/*
ip, user, passwd, port, path string, channel int
*/

func parseDeviceParam(r *http.Request) (*DeviceParam, error) {
	ip := r.URL.Query().Get("ip")
	port := r.URL.Query().Get("port")
	user := r.URL.Query().Get("user")
	passwd := r.URL.Query().Get("passwd")
	path := r.URL.Query().Get("path")
	channel := r.URL.Query().Get("channel")
	iface := r.URL.Query().Get("iface")

	if ip == "" {
		return nil, fmt.Errorf("param ip error")
	}
	if channel == "" {
		channel = "0"
	}
	channelInt, err := strconv.Atoi(channel)
	if err != nil {
		return nil, fmt.Errorf("param channel error")
	}
	if port == "" {
		port = "80"
	}
	if path == "" {
		path = "/onvif/device_service"
	}
	if iface == "" {
		iface = VIRTUAL_IFACE
	}
	return &DeviceParam{
		Ip:      ip,
		Port:    port,
		User:    user,
		Passwd:  passwd,
		Path:    path,
		IFace:   iface,
		Channel: channelInt,
	}, nil
}

func getDevice(autoAdd bool, method string, resp *Response, w http.ResponseWriter, r *http.Request) (*DeviceStatus, error) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != method {
		w.WriteHeader(http.StatusMethodNotAllowed)
		resp.Code = http.StatusMethodNotAllowed
		resp.Msg = "method not allowed"
		json.NewEncoder(w).Encode(resp)
		return nil, fmt.Errorf("method not allowed")
	}

	devParam, err := parseDeviceParam(r)
	if err != nil {
		resp.Code = http.StatusBadRequest
		resp.Msg = err.Error()
		json.NewEncoder(w).Encode(resp)
		return nil, err
	}
	devs, ok := deviceList.Data.Get(devParam.IFace)
	if !ok {
		resp.Code = http.StatusBadRequest
		resp.Msg = "iface not found"
		json.NewEncoder(w).Encode(resp)
		return nil, fmt.Errorf("iface not found")
	}
	dev, ok := devs.Get(devParam.Ip + ":" + devParam.Port)
	if !ok {
		if !autoAdd {
			resp.Code = http.StatusBadRequest
			resp.Msg = "device not found"
			json.NewEncoder(w).Encode(resp)
			return nil, fmt.Errorf("device not found")
		}
		//add device
		ds, code, err := deviceList.AddDevice(devParam)
		if err != nil {
			resp.Msg = err.Error()
			resp.Code = code
			json.NewEncoder(w).Encode(resp)
			return nil, err
		}
		dev = ds
	}
	if devParam.Channel > len(dev.Profiles) {
		resp.Code = http.StatusBadRequest
		resp.Msg = "channel out of range"
		json.NewEncoder(w).Encode(resp)
		return nil, fmt.Errorf("channel out of range")
	}
	dev.Channel = devParam.Channel
	return dev, nil
}

func (o *OnvifPlugin) closeStream(resp *Response, w http.ResponseWriter, r *http.Request) error {
	d, err := getDevice(o.AutoAdd, http.MethodPost, resp, w, r)
	if err != nil {
		return err
	}
	devParam, _ := parseDeviceParam(r)

	streamPath := GenStreamPath(d.Device, util.ConvertRuneToEn(devParam.IFace))

	stream, _ := o.Server.Streams.Get(streamPath)
	if stream == nil {
		return nil
	}
	stream.Stop(errors.New("close manual"))
	return nil
}

func (o *OnvifPlugin) API_list(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	resp := Response{
		Code: 0,
		Msg:  "ok",
		Data: nil,
	}
	byList := r.URL.Query().Get("bylist")
	if byList == "1" {
		list := make([]*DeviceStatus, 0)
		deviceList.Data.Range(func(ic *InterfaceCollection) bool {
			ic.Range(func(ds *DeviceStatus) bool {
				list = append(list, ds)
				return true
			})
			return true
		})
		resp.Data = list
	} else {
		data := make(map[string]map[string]*DeviceStatus)
		deviceList.Data.Range(func(ic *InterfaceCollection) bool {
			if _, ok := data[ic.iface]; !ok {
				data[ic.iface] = make(map[string]*DeviceStatus)
			}
			ic.Range(func(ds *DeviceStatus) bool {
				data[ic.iface][ds.GetKey()] = ds
				return true
			})
			return true
		})
		resp.Data = data
	}

	json.NewEncoder(w).Encode(resp)
	return
}

func (o *OnvifPlugin) API_adddevice(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Content-Type", "application/json")
	resp := Response{
		Code: 0,
		Msg:  "ok",
		Data: map[string]any{},
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		resp.Code = http.StatusMethodNotAllowed
		resp.Msg = "method not allowed"
		json.NewEncoder(w).Encode(resp)
		return
	}

	devParam, err := parseDeviceParam(r)
	if err != nil {
		resp.Code = http.StatusBadRequest
		resp.Msg = err.Error()
		json.NewEncoder(w).Encode(resp)
		return
	}
	//add device
	_, code, err := deviceList.AddDevice(devParam)
	if err != nil {
		resp.Msg = err.Error()
		resp.Code = code
		json.NewEncoder(w).Encode(resp)
		return
	}
	resp.Code = 0
	json.NewEncoder(w).Encode(resp)
	return

}

func (o *OnvifPlugin) API_deldevice(w http.ResponseWriter, r *http.Request) {

	w.Header().Set("Content-Type", "application/json")
	resp := Response{
		Code: 0,
		Msg:  "ok",
		Data: map[string]any{},
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		resp.Code = http.StatusMethodNotAllowed
		resp.Msg = "method not allowed"
		json.NewEncoder(w).Encode(resp)
		return
	}

	devParam, err := parseDeviceParam(r)
	if err != nil {
		resp.Code = http.StatusBadRequest
		resp.Msg = err.Error()
		json.NewEncoder(w).Encode(resp)
		return
	}
	//先关闭流
	err = o.closeStream(&resp, w, r)
	if err != nil {
		resp.Code = http.StatusBadRequest
		resp.Msg = err.Error()
		json.NewEncoder(w).Encode(resp)
		return
	}
	//add device
	deviceList.DelDevice(devParam)
	resp.Code = 0
	json.NewEncoder(w).Encode(resp)
	return

}

func (o *OnvifPlugin) API_pull(w http.ResponseWriter, r *http.Request) {
	resp := Response{
		Code: 0,
		Msg:  "ok",
		Data: map[string]any{},
	}
	d, err := getDevice(o.AutoAdd, http.MethodGet, &resp, w, r)
	if err != nil {
		resp.Code = http.StatusBadRequest
		resp.Msg = err.Error()
		json.NewEncoder(w).Encode(resp)
		return
	}
	devParam, _ := parseDeviceParam(r)

	err = d.PullStream(devParam.IFace, devParam.Channel)

	if err != nil {
		resp.Code = StatusInitError
		resp.Msg = err.Error()
		json.NewEncoder(w).Encode(resp)
		return
	}
	resp.Data = map[string]any{
		"stream": d.Stream,
	}
	json.NewEncoder(w).Encode(resp)

}

func (o *OnvifPlugin) API_close(w http.ResponseWriter, r *http.Request) {
	resp := Response{
		Code: 0,
		Msg:  "ok",
		Data: map[string]any{},
	}

	o.closeStream(&resp, w, r)
	json.NewEncoder(w).Encode(resp)
	return
}

func (o *OnvifPlugin) API_status(w http.ResponseWriter, r *http.Request) {
	resp := Response{
		Code: 0,
		Msg:  "ok",
		Data: map[string]any{},
	}
	dev, err := getDevice(o.AutoAdd, http.MethodGet, &resp, w, r)
	if err != nil {
		return
	}
	resp.Data = map[string]any{
		"status": dev.Status,
	}
	json.NewEncoder(w).Encode(resp)
}

// 获取设备能力
func (o *OnvifPlugin) API_capability(w http.ResponseWriter, r *http.Request) {
	resp := Response{
		Code: 0,
		Msg:  "ok",
		Data: map[string]any{},
	}
	dev, err := getDevice(o.AutoAdd, http.MethodGet, &resp, w, r)
	if err != nil {
		return
	}

	caps := dev.Device.GetServices()
	resp.Data = caps
	json.NewEncoder(w).Encode(resp)
}

// 获取图片能力信息
func (o *OnvifPlugin) API_imageProfile(w http.ResponseWriter, r *http.Request) {
	resp := Response{
		Code: 0,
		Msg:  "ok",
		Data: map[string]any{},
	}
	dev, err := getDevice(o.AutoAdd, http.MethodGet, &resp, w, r)
	if err != nil {
		return
	}

	settings, err := dev.GetImaging()
	if err != nil {
		resp.Code = http.StatusBadRequest
		resp.Msg = err.Error()
		json.NewEncoder(w).Encode(resp)
		return
	}

	resp.Data = settings
	json.NewEncoder(w).Encode(resp)
}

func (o *OnvifPlugin) API_setImageProfile(w http.ResponseWriter, r *http.Request) {
	resp := Response{
		Code: 0,
		Msg:  "ok",
		Data: map[string]any{},
	}
	dev, err := getDevice(o.AutoAdd, http.MethodPost, &resp, w, r)
	if err != nil {
		return
	}
	var settingReq ImageSettingReq
	content, err := io.ReadAll(r.Body)
	if err != nil {
		resp.Code = http.StatusBadRequest
		resp.Msg = err.Error()
		json.NewEncoder(w).Encode(resp)
		return
	}
	err = json.Unmarshal(content, &settingReq)
	if err != nil {
		resp.Code = http.StatusBadRequest
		resp.Msg = err.Error()
		json.NewEncoder(w).Encode(resp)
		return
	}
	var settings onvif.ImagingSettings20
	copier.Copy(&settings, settingReq.ImageSettings)
	err = dev.SetImaging(&settings, settingReq.ForcePersistence)
	if err != nil {
		resp.Code = http.StatusBadRequest
		resp.Msg = err.Error()
		json.NewEncoder(w).Encode(resp)
		return
	}

	json.NewEncoder(w).Encode(resp)
}

// 获取预置位
func (o *OnvifPlugin) API_ptzPreset(w http.ResponseWriter, r *http.Request) {
	resp := Response{
		Code: 0,
		Msg:  "ok",
		Data: map[string]any{},
	}
	dev, err := getDevice(o.AutoAdd, http.MethodGet, &resp, w, r)
	if err != nil {
		return
	}

	settings, err := dev.GetPtzPreset()
	if err != nil {
		resp.Code = http.StatusBadRequest
		resp.Msg = err.Error()
		json.NewEncoder(w).Encode(resp)
		return
	}

	resp.Data = settings
	json.NewEncoder(w).Encode(resp)
}

// 设置预置位 name 必填 preset_token 可选，如果提供则更新预置位
func (o *OnvifPlugin) API_setPtzPreset(w http.ResponseWriter, r *http.Request) {
	resp := Response{
		Code: 0,
		Msg:  "ok",
		Data: map[string]any{},
	}
	dev, err := getDevice(o.AutoAdd, http.MethodPost, &resp, w, r)
	if err != nil {
		return
	}

	name := r.URL.Query().Get("name")
	if name == "" {
		resp.Code = http.StatusBadRequest
		resp.Msg = "name is empty"
		json.NewEncoder(w).Encode(resp)
		return
	}
	presetToken := r.URL.Query().Get("preset_token")
	token, err := dev.SetPtzPreset(name, presetToken)
	if err != nil {
		resp.Code = http.StatusBadRequest
		resp.Msg = err.Error()
		json.NewEncoder(w).Encode(resp)
		return
	}

	resp.Data = map[string]any{"Token": token}
	json.NewEncoder(w).Encode(resp)
}

// 预置位移动
func (o *OnvifPlugin) API_gotoPtzPreset(w http.ResponseWriter, r *http.Request) {
	resp := Response{
		Code: 0,
		Msg:  "ok",
		Data: map[string]any{},
	}
	dev, err := getDevice(o.AutoAdd, http.MethodPost, &resp, w, r)
	if err != nil {
		return
	}

	presetToken := r.URL.Query().Get("preset_token")
	if presetToken == "" {
		resp.Code = http.StatusBadRequest
		resp.Msg = "preset_token is empty"
		json.NewEncoder(w).Encode(resp)
		return
	}
	content, err := io.ReadAll(r.Body)
	if err != nil {
		resp.Code = http.StatusBadRequest
		resp.Msg = err.Error()
		json.NewEncoder(w).Encode(resp)
		return
	}
	var speed *onvif.PTZSpeed
	if string(content) != "" {
		json.Unmarshal(content, &speed)
	}

	err = dev.GotoPtzPreset(presetToken, speed)
	if err != nil {
		resp.Code = http.StatusBadRequest
		resp.Msg = err.Error()
		json.NewEncoder(w).Encode(resp)
		return
	}

	json.NewEncoder(w).Encode(resp)
}

// 预置位删除
func (o *OnvifPlugin) API_removePtzPreset(w http.ResponseWriter, r *http.Request) {
	resp := Response{
		Code: 0,
		Msg:  "ok",
		Data: map[string]any{},
	}
	dev, err := getDevice(o.AutoAdd, http.MethodPost, &resp, w, r)
	if err != nil {
		return
	}

	presetToken := r.URL.Query().Get("preset_token")
	if presetToken == "" {
		resp.Code = http.StatusBadRequest
		resp.Msg = "preset_token is empty"
		json.NewEncoder(w).Encode(resp)
		return
	}

	err = dev.RemovePtzPreset(presetToken)
	if err != nil {
		resp.Code = http.StatusBadRequest
		resp.Msg = err.Error()
		json.NewEncoder(w).Encode(resp)
		return
	}

	json.NewEncoder(w).Encode(resp)
}

// 移动摄像头
func (o *OnvifPlugin) API_ptz(w http.ResponseWriter, r *http.Request) {
	resp := Response{
		Code: 0,
		Msg:  "ok",
		Data: map[string]any{},
	}
	dev, err := getDevice(o.AutoAdd, http.MethodPost, &resp, w, r)
	if err != nil {
		return
	}

	mode := r.URL.Query().Get("mode")
	if mode == "" {
		resp.Code = http.StatusBadRequest
		resp.Msg = "mode is empty"
		json.NewEncoder(w).Encode(resp)
		return
	}
	modeInt, err := strconv.Atoi(mode)
	if err != nil {
		resp.Code = http.StatusBadRequest
		resp.Msg = "mode is error"
		json.NewEncoder(w).Encode(resp)
		return
	}
	content, err := io.ReadAll(r.Body)
	if err != nil {
		resp.Code = http.StatusBadRequest
		resp.Msg = err.Error()
		json.NewEncoder(w).Encode(resp)
		return
	}

	switch modeInt {
	case PtzMoveAbs, PtzMoveRelative:
		var ptzMove ptzMoveReq
		err = json.Unmarshal(content, &ptzMove)
		if err != nil {
			resp.Code = http.StatusBadRequest
			resp.Msg = err.Error()
			json.NewEncoder(w).Encode(resp)
			return
		}
		err = dev.PtzMove(modeInt, ptzMove.Move, ptzMove.Speed)
	case PtzMoveContinue:
		var ptzContinueMove ptzContinueMoveReq
		err = json.Unmarshal(content, &ptzContinueMove)
		if err != nil {
			resp.Code = http.StatusBadRequest
			resp.Msg = err.Error()
			json.NewEncoder(w).Encode(resp)
			return
		}
		err = dev.PtzContinueMove(ptzContinueMove.Velocity, ptzContinueMove.Timeout)
	default:
		resp.Code = http.StatusBadRequest
		resp.Msg = "mode is error"
		json.NewEncoder(w).Encode(resp)
		return
	}

	if err != nil {
		resp.Code = StatusPtzMove
		resp.Msg = err.Error()
		json.NewEncoder(w).Encode(resp)
		return
	}
	json.NewEncoder(w).Encode(resp)
}

func (p *OnvifPlugin) RegisterHandler() map[string]http.HandlerFunc {
	return map[string]http.HandlerFunc{
		"/list":            p.API_list,
		"/add":             p.API_adddevice,
		"/remove":          p.API_deldevice,
		"/ptz":             p.API_ptz,
		"/ptz/preset/get":  p.API_ptzPreset,
		"/ptz/preset/set":  p.API_setPtzPreset,
		"/ptz/preset/goto": p.API_gotoPtzPreset,
		"/imaging/get":     p.API_imageProfile,
		"/imaging/set":     p.API_setImageProfile,
	}
}
