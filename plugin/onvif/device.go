package plugin_onvif

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"m7s.live/v5/pkg/util"
	"strings"

	donvif "github.com/IOTechSystems/onvif"
	"github.com/IOTechSystems/onvif/gosoap"
	"github.com/IOTechSystems/onvif/imaging"
	"github.com/IOTechSystems/onvif/media"
	"github.com/IOTechSystems/onvif/ptz"
	"github.com/IOTechSystems/onvif/xsd"
	"github.com/IOTechSystems/onvif/xsd/onvif"
	onvifTypes "github.com/IOTechSystems/onvif/xsd/onvif"
	"github.com/beevik/etree"
	"m7s.live/v5/pkg/config"
	rtsp "m7s.live/v5/plugin/rtsp/pkg"
)

// 设备状态常量
const (
	StatusInitOk = iota
	StatusInitError
	StatusAddError
	StatusProfileError
	StatusGetStreamUriOk
	StatusGetStreamUriError
	StatusPullRtspOk
	StatusPullRtspError
	StatusGetImagingSetting
	StatusSetImagingSetting
	StatusGetPtzPreset
	StatusSetPtzPreset
	StatusGotoPtzPreset
	StatusPtzMove
)

// PTZ移动模式
const (
	PtzMoveAbs = iota
	PtzMoveRelative
	PtzMoveContinue
)

type DeviceStatus struct {
	Device      *donvif.Device
	Xaddr       string `json:"xaddr"`       // onvif 设备地址
	IP          string `json:"ip"`          // 设备IP
	Port        string `json:"port"`        // 设备端口
	Username    string `json:"username"`    // 设备用户名
	Password    string `json:"password"`    // 设备密码
	Path        string `json:"path"`        // onvif device_service 路径
	MediaUrl    string `json:"mediaUrl"`    // rtsp 流
	Channel     int    `json:"channel"`     // 设备通道
	Stream      string `json:"stream"`      // 设备流
	Status      int    `json:"status"`      // 设备状态
	Description string `json:"description"` // 状态描述
	Profiles    []onvif.Profile
}

func (d *DeviceStatus) GetKey() string {
	if d.Xaddr == "" {
		d.Xaddr = d.IP + ":" + d.Port
	}
	return d.Xaddr
}

type AuthConfig struct {
	Interfaces map[string]deviceAuth
	Devices    map[string]deviceAuth
}

type deviceAuth struct {
	Username string
	Password string
}

var authCfg = &AuthConfig{
	Interfaces: make(map[string]deviceAuth),
	Devices:    make(map[string]deviceAuth),
}

func GenStreamPath(device *donvif.Device, ifname string) string {
	streamPath := strings.ReplaceAll(device.GetDeviceParams().Xaddr, ".", "_")
	streamPath = "onvif/" + util.ConvertRuneToEn(ifname) + "/" + strings.ReplaceAll(streamPath, ":", "_")
	return streamPath
}
func NewDeviceStatus(ip, user, passwd, port, path string, channel int) (*DeviceStatus, int, error) {
	param := donvif.DeviceParams{Xaddr: ip + ":" + port, Username: user, Password: passwd, EndpointRefAddress: path}
	device, err := donvif.NewDevice(param)
	if err != nil {
		return nil, StatusAddError, err
	}
	profiles, err := GetProfiles(device)
	if err != nil {
		return nil, StatusProfileError, err
	}
	return &DeviceStatus{Device: device,
		Channel: channel, Path: path, Profiles: profiles,
		IP: ip, Port: port,
		Username: user,
		Password: passwd,
	}, 0, nil

}

// MarshalJSON 实现设备状态的JSON序列化
func (d *DeviceStatus) MarshalJSON() ([]byte, error) {
	type Alias DeviceStatus
	return json.Marshal(&struct {
		*Alias
		Status      int    `json:"status"`
		StatusText  string `json:"status_text"`
		Profiles    int    `json:"profiles,omitempty"`
		StreamToken string `json:"stream_token,omitempty"`
	}{
		Alias:      (*Alias)(d),
		Status:     d.Status,
		StatusText: getStatusText(d.Status),
	})
}

// GetImaging 获取图像设置
func (d *DeviceStatus) GetImaging() (*onvifTypes.ImagingSettings20, error) {
	dev, err := donvif.NewDevice(donvif.DeviceParams{
		Xaddr:    fmt.Sprintf("http://%s/onvif/device_service", d.IP),
		Username: d.Username,
		Password: d.Password,
	})
	if err != nil {
		return nil, err
	}
	if len(d.Profiles) == 0 {
		return nil, fmt.Errorf("no profiles found")
	}
	token := onvifTypes.ReferenceToken(d.Profiles[d.Channel].Token)
	result, err := dev.CallMethod(imaging.GetImagingSettings{
		VideoSourceToken: token,
	})
	if err != nil {
		return nil, err
	}
	contents, err := io.ReadAll(result.Body)
	if err != nil {
		return nil, err
	}
	var imagingSetting imaging.GetImagingSettingsResponse
	responseEnvelope := gosoap.NewSOAPEnvelope(&imagingSetting)
	err = xml.Unmarshal(contents, responseEnvelope)
	if err != nil {
		return nil, err
	}
	return &imagingSetting.ImagingSettings, nil
}

// SetImaging 设置图像参数
func (d *DeviceStatus) SetImaging(settings *onvifTypes.ImagingSettings20, forcePersistence bool) error {
	dev, err := donvif.NewDevice(donvif.DeviceParams{
		Xaddr:    fmt.Sprintf("http://%s/onvif/device_service", d.IP),
		Username: d.Username,
		Password: d.Password,
	})
	if err != nil {
		return err
	}
	if len(d.Profiles) == 0 {
		return fmt.Errorf("no profiles found")
	}
	token := onvifTypes.ReferenceToken(d.Profiles[d.Channel].Token)
	result, err := dev.CallMethod(imaging.SetImagingSettings{
		VideoSourceToken: token,
		ImagingSettings:  *settings,
		ForcePersistence: xsd.Boolean(forcePersistence),
	})
	if err != nil {
		return err
	}
	contents, err := io.ReadAll(result.Body)
	if err != nil {
		return err
	}
	var imagingSetting imaging.SetImagingSettingsResponse
	responseEnvelope := gosoap.NewSOAPEnvelope(&imagingSetting)
	err = xml.Unmarshal(contents, responseEnvelope)
	if err != nil {
		return err
	}
	return nil
}

// GetPtzPresets 获取预置点列表
func (d *DeviceStatus) GetPtzPresets() ([]onvifTypes.PTZPreset, error) {
	dev, err := donvif.NewDevice(donvif.DeviceParams{
		Xaddr:    fmt.Sprintf("http://%s/onvif/device_service", d.IP),
		Username: d.Username,
		Password: d.Password,
	})
	if err != nil {
		return nil, err
	}
	if len(d.Profiles) == 0 {
		return nil, fmt.Errorf("no profiles found")
	}
	token := onvifTypes.ReferenceToken(d.Profiles[d.Channel].Token)
	result, err := dev.CallMethod(ptz.GetPresets{
		ProfileToken: token,
	})
	if err != nil {
		return nil, err
	}
	contents, err := io.ReadAll(result.Body)
	if err != nil {
		return nil, err
	}
	var presets ptz.GetPresetsResponse
	responseEnvelope := gosoap.NewSOAPEnvelope(&presets)
	err = xml.Unmarshal(contents, responseEnvelope)
	if err != nil {
		return nil, err
	}
	return presets.Preset, nil
}

// SetPtzPreset 设置预置点
func (d *DeviceStatus) SetPtzPreset(name string, presetToken string) (*onvif.ReferenceToken, error) {
	token := d.Profiles[d.Channel].Token
	ptzName := xsd.String(name)
	ptzPresetToken := onvif.ReferenceToken(presetToken)
	result, err := d.Device.CallMethod(ptz.SetPreset{
		ProfileToken: &token,
		PresetToken:  &ptzPresetToken,
		PresetName:   &ptzName,
	})
	if err != nil {
		return nil, err
	}
	contents, err := io.ReadAll(result.Body)
	if err != nil {
		return nil, err
	}
	var presets ptz.SetPresetResponse
	responseEnvelope := gosoap.NewSOAPEnvelope(&presets)
	err = xml.Unmarshal(contents, responseEnvelope)
	if err != nil {
		return nil, err
	}
	return &presets.PresetToken, nil
}

// PtzMove PTZ移动控制
func (d *DeviceStatus) PtzMove(mode int, move ptz.Vector, speed ptz.Speed) error {
	dev, err := donvif.NewDevice(donvif.DeviceParams{
		Xaddr:    fmt.Sprintf("http://%s/onvif/device_service", d.IP),
		Username: d.Username,
		Password: d.Password,
	})
	if err != nil {
		return err
	}
	if len(d.Profiles) == 0 {
		return fmt.Errorf("no profiles found")
	}
	token := onvifTypes.ReferenceToken(d.Profiles[d.Channel].Token)

	switch mode {
	case PtzMoveAbs:
		result, err := dev.CallMethod(ptz.AbsoluteMove{
			ProfileToken: token,
			Position:     move,
			Speed:        speed,
		})
		if err != nil {
			return err
		}
		contents, err := io.ReadAll(result.Body)
		if err != nil {
			return err
		}
		var moveResponse ptz.AbsoluteMoveResponse
		responseEnvelope := gosoap.NewSOAPEnvelope(&moveResponse)
		return xml.Unmarshal(contents, responseEnvelope)
	case PtzMoveRelative:
		result, err := dev.CallMethod(ptz.RelativeMove{
			ProfileToken: token,
			Translation:  move,
			Speed:        speed,
		})
		if err != nil {
			return err
		}
		contents, err := io.ReadAll(result.Body)
		if err != nil {
			return err
		}
		var moveResponse ptz.RelativeMoveResponse
		responseEnvelope := gosoap.NewSOAPEnvelope(&moveResponse)
		return xml.Unmarshal(contents, responseEnvelope)
	case PtzMoveContinue:
		result, err := dev.CallMethod(ptz.ContinuousMove{
			ProfileToken: &token,
			Velocity: &onvifTypes.PTZSpeed{
				PanTilt: move.PanTilt,
				Zoom:    move.Zoom,
			},
		})
		if err != nil {
			return err
		}
		contents, err := io.ReadAll(result.Body)
		if err != nil {
			return err
		}
		var moveResponse ptz.ContinuousMoveResponse
		responseEnvelope := gosoap.NewSOAPEnvelope(&moveResponse)
		return xml.Unmarshal(contents, responseEnvelope)
	default:
		return fmt.Errorf("unknown ptz move mode: %d", mode)
	}
}

// GotoPtzPreset 跳转到预置点
func (d *DeviceStatus) GotoPtzPreset(presetToken string, speed *onvifTypes.PTZSpeed) error {
	dev, err := donvif.NewDevice(donvif.DeviceParams{
		Xaddr:    fmt.Sprintf("http://%s/onvif/device_service", d.IP),
		Username: d.Username,
		Password: d.Password,
	})
	if err != nil {
		return err
	}
	if len(d.Profiles) == 0 {
		return fmt.Errorf("no profiles found")
	}
	token := onvifTypes.ReferenceToken(d.Profiles[d.Channel].Token)
	ptzPresetToken := onvifTypes.ReferenceToken(presetToken)
	result, err := dev.CallMethod(ptz.GotoPreset{
		ProfileToken: &token,
		PresetToken:  &ptzPresetToken,
		Speed:        speed,
	})
	if err != nil {
		return err
	}
	contents, err := io.ReadAll(result.Body)
	if err != nil {
		return err
	}
	var presets ptz.GotoPresetResponse
	responseEnvelope := gosoap.NewSOAPEnvelope(&presets)
	err = xml.Unmarshal(contents, responseEnvelope)
	if err != nil {
		return err
	}
	return nil
}

func (d *DeviceStatus) PullStream(ifname string, channel int) error {
	// 生成流路径
	streamPath := GenStreamPath(d.Device, ifname)
	var rtspUrl string
	var err error
	if d.MediaUrl != "" {
		rtspUrl = d.MediaUrl
	} else {
		// 获取 RTSP 流地址
		rtspUrl, err = GetStreamUri(d.Device, d.Profiles[channel].Token)
		if err != nil {
			d.Status = StatusGetStreamUriError
			return fmt.Errorf("get stream uri error: %v", err)
		}
		d.Status = StatusGetStreamUriOk
	}

	pullConf := config.Pull{
		URL: rtspUrl,
	}
	// pubConf := config.Publish{}
	puller := rtsp.NewPuller(pullConf)
	puller.GetPullJob().Init(puller, &deviceList.plugin.Plugin, streamPath, pullConf, nil)
	d.Status = StatusPullRtspOk
	d.Stream = streamPath
	return nil
}

func (d *DeviceStatus) GetPtzPreset() ([]onvif.PTZPreset, error) {
	token := d.Profiles[d.Channel].Token
	result, err := d.Device.CallMethod(ptz.GetPresets{ProfileToken: token})
	if err != nil {
		return nil, err
	}
	contents, err := io.ReadAll(result.Body)
	if err != nil {
		return nil, err
	}
	var presets ptz.GetPresetsResponse
	responseEnvelope := gosoap.NewSOAPEnvelope(&presets)
	err = xml.Unmarshal(contents, responseEnvelope)
	if err != nil {
		return nil, err
	}
	return presets.Preset, nil
}

func (d *DeviceStatus) RemovePtzPreset(presetToken string) error {
	ptzPresetToken := onvif.ReferenceToken(presetToken)
	token := d.Profiles[d.Channel].Token
	result, err := d.Device.CallMethod(ptz.RemovePreset{
		ProfileToken: token,
		PresetToken:  ptzPresetToken,
	})
	if err != nil {
		return err
	}
	contents, err := io.ReadAll(result.Body)
	if err != nil {
		return err
	}
	var presets ptz.RemovePresetResponse
	responseEnvelope := gosoap.NewSOAPEnvelope(&presets)
	err = xml.Unmarshal(contents, responseEnvelope)
	if err != nil {
		return err
	}
	return nil
}

func (d *DeviceStatus) PtzContinueMove(vel *onvif.PTZSpeed, tmout *xsd.Duration) error {
	token := d.Profiles[d.Channel].Token

	result, err := d.Device.CallMethod(ptz.ContinuousMove{
		ProfileToken: &token,
		Velocity:     vel,
		Timeout:      tmout,
	})
	if err != nil {
		return err
	}
	contents, err := io.ReadAll(result.Body)
	if err != nil {
		return err
	}
	var resp ptz.ContinuousMoveResponse
	responseEnvelope := gosoap.NewSOAPEnvelope(&resp)
	err = xml.Unmarshal(contents, responseEnvelope)
	if err != nil {
		return err
	}
	return nil
}

func getStatusText(status int) string {
	switch status {
	case StatusInitOk:
		return "初始化成功"
	case StatusInitError:
		return "初始化失败"
	case StatusAddError:
		return "添加设备失败"
	case StatusProfileError:
		return "获取配置文件失败"
	case StatusGetStreamUriOk:
		return "获取流地址成功"
	case StatusGetStreamUriError:
		return "获取流地址失败"
	case StatusPullRtspOk:
		return "拉流成功"
	case StatusPullRtspError:
		return "拉流失败"
	case StatusGetImagingSetting:
		return "获取图像设置"
	case StatusSetImagingSetting:
		return "设置图像参数"
	case StatusGetPtzPreset:
		return "获取预置点"
	case StatusSetPtzPreset:
		return "设置预置点"
	case StatusGotoPtzPreset:
		return "跳转预置点"
	case StatusPtzMove:
		return "云台控制"
	default:
		return "未知状态"
	}
}

func GetStreamUri(dev *donvif.Device, profileToken onvifTypes.ReferenceToken) (string, error) {

	response, err := dev.CallMethod(media.GetStreamUri{ProfileToken: &profileToken})
	if err != nil {
		return "", err
	}
	resp, err := io.ReadAll(response.Body)
	if err != nil {
		return "", err
	}

	doc := etree.NewDocument()

	if err := doc.ReadFromBytes(resp); err != nil {
		return "", fmt.Errorf("error:%s", err.Error())
	}

	endpoints := doc.Root().FindElements("./Body/GetStreamUriResponse/MediaUri/Uri")
	if len(endpoints) == 0 {
		return "", fmt.Errorf("error:%s", "no media uri")
	}
	mediaUri := endpoints[0].Text()
	if !strings.Contains(mediaUri, "rtsp") {
		fmt.Println("mediaUri:", mediaUri)
		return "", fmt.Errorf("error:%s", "media uri is not rtsp")
	}
	if !strings.Contains(mediaUri, "@") && dev.GetDeviceParams().Username != "" {
		//如果返回的rtsp里没有账号密码，则自己拼接
		mediaUri = strings.Replace(mediaUri, "//", fmt.Sprintf("//%s:%s@", dev.GetDeviceParams().Username, dev.GetDeviceParams().Password), 1)
	}
	if strings.Contains(mediaUri, "udp") {
		mediaUri = strings.Replace(mediaUri, "udp", "rtsp", 1)
	}
	return mediaUri, nil
}

// 获取设备的账号密码
func getDeviceAuth(interfaceName string, ip string, config *AuthConfig) deviceAuth {
	var auth deviceAuth
	if a, ok := config.Interfaces[interfaceName]; ok {
		auth = a
	}
	if a, ok := config.Devices[ip]; ok {
		auth = a
	}
	return auth
}

func GetProfiles(dev *donvif.Device) ([]onvifTypes.Profile, error) {
	result, err := dev.CallMethod(media.GetProfiles{})
	if err != nil {
		return nil, err
	}
	contents, err := io.ReadAll(result.Body)
	if err != nil {
		return nil, err
	}
	var profiles media.GetProfilesResponse
	responseEnvelope := gosoap.NewSOAPEnvelope(&profiles)
	err = xml.Unmarshal(contents, responseEnvelope)
	if err != nil {
		return nil, err
	}
	return profiles.Profiles, nil
}

func getAuth(iface, ip string) *deviceAuth {
	if auth, ok := authCfg.Devices[ip]; ok {
		return &auth
	}
	if auth, ok := authCfg.Interfaces[iface]; ok {
		return &auth
	}
	return nil
}
