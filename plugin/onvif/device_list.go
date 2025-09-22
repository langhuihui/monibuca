package plugin_onvif

import (
	"encoding/xml"
	"fmt"
	"net/url"
	"strings"
	"sync"

	"github.com/kerberos-io/onvif"
	wsdiscovery "github.com/kerberos-io/onvif/ws-discovery"
	"m7s.live/v5/pkg/util"
)

type DeviceList struct {
	Data   *util.Collection[string, *InterfaceCollection]
	plugin *OnvifPlugin
}

var deviceList DeviceList

func (d *DeviceList) discoveryDevice() {
	for _, iface := range d.plugin.Interfaces {
		if iface.InterfaceName == VIRTUAL_IFACE {
			continue
		}
		devices, err := wsdiscovery.SendProbe(iface.InterfaceName, nil, []string{"dn:NetworkVideoTransmitter"}, map[string]string{"dn": "http://www.onvif.org/ver10/network/wsdl"})
		if err != nil {
			d.plugin.Warn("discover devices failed",
				"interface", iface.InterfaceName,
				"error", err.Error(),
			)
			continue
		}

		var ifaceDevices *InterfaceCollection
		var ok bool
		if ifaceDevices, ok = d.Data.Get(iface.InterfaceName); !ok {
			ifaceDevices = &InterfaceCollection{
				Collection: util.Collection[string, *DeviceStatus]{
					Items: make([]*DeviceStatus, 0),
					L:     &sync.RWMutex{},
				},
				iface: iface.InterfaceName,
			}
		}

		d.Data.Add(ifaceDevices)

		for _, dev := range devices {
			if strings.Contains(dev, "onvif") {
				var envelope struct {
					Body struct {
						ProbeMatches struct {
							ProbeMatch struct {
								XAddrs string `xml:"XAddrs"`
							} `xml:"ProbeMatch"`
						} `xml:"ProbeMatches"`
					} `xml:"Body"`
				}
				if err := xml.Unmarshal([]byte(dev), &envelope); err != nil {
					d.plugin.Warn("parse device xml failed", "error", err.Error())
					continue
				}

				xaddr := envelope.Body.ProbeMatches.ProbeMatch.XAddrs
				if xaddr == "" {
					continue
				}

				u, err := url.Parse(xaddr)
				if err != nil {
					d.plugin.Warn("parse xaddr failed", "error", err.Error())
					continue
				}

				ipPort := strings.Split(u.Host, ":")
				if len(ipPort) == 1 {
					ipPort = append(ipPort, "80")
				}
				if _, ok := ifaceDevices.Get(u.Host); !ok {
					auth := getAuth(iface.InterfaceName, ipPort[0])
					if auth == nil {
						continue
					}
					status, _, err := NewDeviceStatus(ipPort[0], auth.Username, auth.Password, ipPort[1], "/onvif/device_service", 0)
					if err != nil {
						d.plugin.Warn("create device failed", "error", err.Error())
						continue
					}
					ifaceDevices.Add(status)
					if d.plugin.AutoAdd {
						status.AutoAdd()
					}
				}
			}
		}
	}
}

func (d *DeviceList) AutoPullStream() {
	d.Data.Range(func(ic *InterfaceCollection) bool {
		ic.Range(func(status *DeviceStatus) bool {
			if status.Stream == "" && status.MediaUrl != "" {
				status.PullStream(ic.iface, status.Channel)
			}
			return true
		})
		return true
	})
}

func (d *DeviceList) GetDeviceByStreamPath(streamPath string) *DeviceStatus {
	if streamPath == "" {
		return nil
	}
	var result *DeviceStatus
	d.Data.Range(func(ic *InterfaceCollection) bool {
		ic.Range(func(status *DeviceStatus) bool {
			if status.Path == streamPath {
				result = status
				return false
			}
			return true
		})
		if result != nil {
			return false
		}
		return true
	})
	return result
}

func (d *DeviceStatus) AutoAdd() error {
	dev, err := onvif.NewDevice(onvif.DeviceParams{
		Xaddr:    fmt.Sprintf("%s:%s", d.IP, d.Port),
		Username: d.Username,
		Password: d.Password,
	})
	if err != nil {
		return err
	}

	profiles, err := GetProfiles(dev)
	if err != nil {
		return err
	}

	for i, p := range profiles {
		uri, err := GetStreamUri(dev, p.Token)
		if err != nil {
			continue
		}
		d.MediaUrl = string(uri)
		d.Channel = i
		break
	}
	return nil
}

func (dl *DeviceList) AddDevice(param *DeviceParam) (*DeviceStatus, int, error) {
	if param.Port == "" {
		param.Port = "80"
	}
	if param.Path == "" {
		param.Path = "/onvif/device_service"
	}
	if param.IFace == "" {
		param.IFace = VIRTUAL_IFACE
	}
	xaddr := param.Ip + ":" + param.Port
	devs, ok := dl.Data.Get(param.IFace)
	if ok {
		if dev, exist := devs.Get(xaddr); exist {
			return dev, 0, nil
		}
	} else {
		devs = &InterfaceCollection{iface: param.IFace}
	}
	ds, code, err := NewDeviceStatus(param.Ip, param.User, param.Passwd, param.Port, param.Path, param.Channel)
	if err != nil {
		return nil, code, err
	}
	if !devs.Set(ds) {
		return nil, 0, fmt.Errorf("failed to add device %s", param.IFace)
	}
	dl.Data.Set(devs)
	return ds, 0, nil
}
func (dl *DeviceList) DelDevice(param *DeviceParam) {
	devs, ok := dl.Data.Get(param.IFace)
	if !ok {
		return
	}
	devs.RemoveByKey(param.Ip + ":" + param.Port)
}
