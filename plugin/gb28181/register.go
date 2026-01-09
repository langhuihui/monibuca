package plugin_gb28181pro

import (
	"errors"
	"time"

	task "github.com/langhuihui/gotask"
)

type Register struct {
	task.TickTask
	platform              *Platform
	registerType          string
	platformKeepAliveTask *PlatformKeepAliveTask
}

func NewRegister(platform *Platform, registerType string) *Register {
	register := &Register{
		registerType: registerType,
	}
	register.platform = platform
	return register
}

func (r *Register) GetTickInterval() time.Duration {
	if r.registerType == "firstRegister" {
		return time.Second * time.Duration(r.platform.PlatformModel.RegisterInterval)
	} else {
		return time.Second * time.Duration(r.platform.PlatformModel.Expires)
	}
}

func (r *Register) Tick(any) {
	go r.Register()
}

func (r *Register) Register() {
	if err := r.platform.DoRegister(); err != nil {
		if r.registerType == "keepaliveRegister" { //保活注册失败，需要回到首次注册类型
			r.Error("keepaliveRegister err", err, "register type is ", r.registerType, "DeviceGBId is", r.platform.PlatformModel.DeviceGBID)
			//r.platform.eventChan <- r
			r.registerType = "firstRegister"
			r.platformKeepAliveTask.Stop(errors.New("keepaliveRegister failed,start to firstRegister,DeviceGBId is" + r.platform.PlatformModel.DeviceGBID))
			r.Ticker.Reset(time.Second * time.Duration(r.platform.PlatformModel.RegisterInterval))
			//register := NewRegister(r.platform, "firstRegister")
			//r.platform.AddTask(register)
			//r.Stop(errors.New("keepaliveRegister failed,start to firstRegister,DeviceGBId is" + r.platform.PlatformModel.DeviceGBID))
		}
	} else {
		if r.registerType == "firstRegister" {
			r.platform.Info("firstRegister success", "register type is ", r.registerType, "DeviceGBId is", r.platform.PlatformModel.DeviceGBID)
			//r.platform.eventChan <- r
			//register := NewRegister(r.platform, "keepaliveRegister")
			//r.platform.AddTask(register)

			r.registerType = "keepaliveRegister"
			pat := PlatformKeepAliveTask{
				platform: r.platform,
			}
			pat.Logger = r.platform.plugin.Logger.With("platform_server_gb_id", r.platform.PlatformModel.ServerGBID)
			r.platformKeepAliveTask = &pat
			r.platform.AddTask(&pat)
			r.Ticker.Reset(time.Second * time.Duration(r.platform.PlatformModel.Expires))
			//r.Stop(errors.New("firstRegister success,start to keepaliveRegister,DeviceGBId is" + r.platform.PlatformModel.DeviceGBID))
		}
	}
}

//func (r *Register) Dispose() {
//	r.platform.Info("into dispose,DeviceGBId is", r.platform.PlatformModel.DeviceGBID)
//}
