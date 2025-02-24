package plugin_gb28181pro

import (
	"context"
	"errors"
	"time"

	"m7s.live/v5/pkg/task"
)

type Register struct {
	task.TickTask
	platform              *Platform
	registerType          string
	seconds               time.Duration
	platformKeepAliveTask *PlatformKeepAliveTask
}

func NewRegister(platform *Platform, registerType string) *Register {
	register := &Register{
		registerType: registerType,
	}
	register.platform = platform
	if registerType == "firstRegister" {
		register.seconds = time.Second * 60
	} else {
		register.seconds = time.Second * time.Duration(platform.PlatformModel.Expires)
	}
	return register
}

func (r *Register) GetTickInterval() time.Duration {
	return r.seconds
}

func (r *Register) Tick(any) {
	r.Register()
}

func (r *Register) Register() {
	ctx := context.Background()
	if err := r.platform.DoRegister(ctx); err != nil {
		if r.registerType == "keepaliveRegister" { //保活注册失败，需要回到首次注册类型
			r.Error("keepaliveRegister err", err, "register type is ", r.registerType, "DeviceGBId is", r.platform.PlatformModel.DeviceGBID)
			//r.platform.eventChan <- r
			r.platformKeepAliveTask.Stop(errors.New("keepaliveRegister failed,start to firstRegister,DeviceGBId is" + r.platform.PlatformModel.DeviceGBID))
			r.Ticker.Reset(time.Second * 60)
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

			pat := PlatformKeepAliveTask{
				platform: r.platform,
			}
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
