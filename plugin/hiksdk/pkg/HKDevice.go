package pkg

/*
#cgo CFLAGS:  -I../include

// Linux平台的链接配置
#cgo linux LDFLAGS: -L../lib/Linux -lHCCore -lhpr -lhcnetsdk

// Windows平台的链接配置
#cgo windows LDFLAGS: -L../lib/Windows -lHCCore -lHCNetSDK
#include <stdio.h>
#include <stdlib.h>
#include "HCNetSDK.h"
extern void AlarmCallBack(LONG lCommand, NET_DVR_ALARMER *pAlarmer, char *pAlarmInfo, DWORD dwBufLen, void* pUser);
extern void RealDataCallBack_V30(LONG lRealHandle, DWORD dwDataType, BYTE *pBuffer, DWORD dwBufSize, void *pUser);
*/
import "C"
import (
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/text/encoding/simplifiedchinese"
)

// 全局指针映射，用于安全地在CGO中传递Go指针
var (
	pointerMap     = make(map[uintptr]*Receiver)
	pointerMutex   sync.RWMutex
	pointerCounter uintptr = 1
)

// 存储Go指针，返回一个唯一的标识符
func storePointer(receiver *Receiver) uintptr {
	pointerMutex.Lock()
	defer pointerMutex.Unlock()
	id := pointerCounter
	pointerCounter++
	pointerMap[id] = receiver
	return id
}

// 根据标识符获取Go指针
func getPointer(id uintptr) *Receiver {
	pointerMutex.RLock()
	defer pointerMutex.RUnlock()
	return pointerMap[id]
}

// 删除存储的指针
func removePointer(id uintptr) {
	pointerMutex.Lock()
	defer pointerMutex.Unlock()
	delete(pointerMap, id)
}

/*************************参数配置命令 begin*******************************/
//用于NET_DVR_SetDVRConfig和NET_DVR_GetDVRConfig,注意其对应的配置结构
const (
	NET_DVR_GET_DEVICECFG     = 100  // 获取设备参数
	NET_DVR_SET_DEVICECFG     = 101  // 设置设备参数
	NET_DVR_GET_NETCFG        = 102  // 获取网络参数
	NET_DVR_SET_NETCFG        = 103  // 设置网络参数
	NET_DVR_GET_PICCFG        = 104  // 获取图象参数
	NET_DVR_SET_PICCFG        = 105  // 设置图象参数
	NET_DVR_GET_COMPRESSCFG   = 106  // 获取压缩参数
	NET_DVR_SET_COMPRESSCFG   = 107  // 设置压缩参数
	NET_DVR_GET_RECORDCFG     = 108  // 获取录像时间参数
	NET_DVR_SET_RECORDCFG     = 109  // 设置录像时间参数
	NET_DVR_GET_DECODERCFG    = 110  // 获取解码器参数
	NET_DVR_SET_DECODERCFG    = 111  // 设置解码器参数
	NET_DVR_GET_RS232CFG      = 112  // 获取232串口参数
	NET_DVR_SET_RS232CFG      = 113  // 设置232串口参数
	NET_DVR_GET_ALARMINCFG    = 114  // 获取报警输入参数
	NET_DVR_SET_ALARMINCFG    = 115  // 设置报警输入参数
	NET_DVR_GET_ALARMOUTCFG   = 116  // 获取报警输出参数
	NET_DVR_SET_ALARMOUTCFG   = 117  // 设置报警输出参数
	NET_DVR_GET_TIMECFG       = 118  // 获取DVR时间
	NET_DVR_SET_TIMECFG       = 119  // 设置DVR时间
	NET_DVR_GET_PREVIEWCFG    = 120  // 获取预览参数
	NET_DVR_SET_PREVIEWCFG    = 121  // 设置预览参数
	NET_DVR_GET_VIDEOOUTCFG   = 122  // 获取视频输出参数
	NET_DVR_SET_VIDEOOUTCFG   = 123  // 设置视频输出参数
	NET_DVR_GET_USERCFG       = 124  // 获取用户参数
	NET_DVR_SET_USERCFG       = 125  // 设置用户参数
	NET_DVR_GET_EXCEPTIONCFG  = 126  // 获取异常参数
	NET_DVR_SET_EXCEPTIONCFG  = 127  // 设置异常参数
	NET_DVR_GET_ZONEANDDST    = 128  // 获取时区和夏时制参数
	NET_DVR_SET_ZONEANDDST    = 129  // 设置时区和夏时制参数
	NET_DVR_GET_DEVICECFG_V40 = 1100 // 获取设备参数
	NET_DVR_SET_PTZPOS        = 292  //云台设置PTZ位置
	NET_DVR_GET_PTZPOS        = 293  //云台获取PTZ位置
	NET_DVR_GET_PTZSCOPE      = 294  //云台获取PTZ范围
)

// strcpy safely copies a Go string to a C character array
func strcpy(dst unsafe.Pointer, src string, maxLen int) {
	if len(src) >= maxLen {
		copy((*[1 << 30]byte)(dst)[:maxLen-1], src)
		(*[1 << 30]byte)(dst)[maxLen-1] = 0
	} else {
		copy((*[1 << 30]byte)(dst)[:len(src)], src)
		(*[1 << 30]byte)(dst)[len(src)] = 0
	}
}

// GBK → UTF-8
func GBKToUTF8(b []byte) (string, error) {
	r, err := simplifiedchinese.GBK.NewDecoder().Bytes(b)
	return string(r), err
}

// UTF-8 → GBK
func UTF8ToGBK(s string) ([]byte, error) {
	return simplifiedchinese.GBK.NewEncoder().Bytes([]byte(s))
}

//export AlarmCallBack
func AlarmCallBack(command C.LONG, alarm *C.NET_DVR_ALARMER, info *C.char, len C.DWORD, user unsafe.Pointer) {
	fmt.Println("receive alarm")
}

//export RealDataCallBack_V30
func RealDataCallBack_V30(lRealHandle C.LONG, dwDataType C.DWORD, pBuffer *C.BYTE, dwBufSize C.DWORD, pUser unsafe.Pointer) {
	// 从指针映射中获取Receiver
	receiverID := uintptr(pUser)
	receiver := getPointer(receiverID)
	if receiver == nil {
		fmt.Println("Error: receiver not found for ID", receiverID)
		return
	}

	size := int(dwBufSize)
	if size > 0 && pBuffer != nil {
		// 将C指针转换为Go的byte切片
		buffer := (*[1 << 30]C.BYTE)(unsafe.Pointer(pBuffer))[:size:size]
		// 使用unsafe.Pointer高效转换[]C.BYTE为[]byte
		goBuffer := (*[1 << 30]byte)(unsafe.Pointer(&buffer[0]))[:len(buffer):len(buffer)]
		receiver.ReadPSData(goBuffer)
	}
}

type HKDevice struct {
	ip          string
	port        int
	username    string
	password    string
	loginId     int
	alarmHandle int
	lRealHandle int
	byChanNum   int
	receiverID  uintptr // 存储指针映射ID
}

// InitHikSDK hk sdk init
func InitHikSDK() {
	// 初始化SDK
	C.NET_DVR_Init()
	C.NET_DVR_SetConnectTime(2000, 5)
	C.NET_DVR_SetReconnect(10000, 1)
}

// HKExit hk sdk clean
func HKExit() {
	C.NET_DVR_Cleanup()
}

// NewHKDevice new hk-device instance
func NewHKDevice(info DeviceInfo) Device {
	return &HKDevice{
		ip:       info.IP,
		port:     info.Port,
		username: info.UserName,
		password: info.Password}
}

// Login hk device loin
func (device *HKDevice) Login() (int, error) {
	// init data
	var deviceInfoV30 C.NET_DVR_DEVICEINFO_V30
	ip := C.CString(device.ip)
	usr := C.CString(device.username)
	passwd := C.CString(device.password)
	defer func() {
		C.free(unsafe.Pointer(ip))
		C.free(unsafe.Pointer(usr))
		C.free(unsafe.Pointer(passwd))
	}()

	device.loginId = int(C.NET_DVR_Login_V30(ip, C.WORD(device.port), usr, passwd,
		(*C.NET_DVR_DEVICEINFO_V30)(unsafe.Pointer(&deviceInfoV30)),
	))
	// 正确地将_Ctype_BYTE数组转换为string
	serialNumber := string(unsafe.Slice(&deviceInfoV30.sSerialNumber[0], len(deviceInfoV30.sSerialNumber)))
	// 去除可能的空字符
	serialNumber = strings.Trim(serialNumber, "\x00")
	fmt.Println("设备序列号:", serialNumber)
	fmt.Println("登录成功，设备信息已获取")
	if device.loginId < 0 {
		return -1, device.HKErr("login ")
	}
	log.Println("login success")
	return device.loginId, nil
}

// Logout hk device logout
func (device *HKDevice) Logout() error {
	C.NET_DVR_Logout_V30(C.LONG(device.loginId))
	if err := device.HKErr("NVRLogout"); err != nil {
		return err
	}
	return nil
}

// Login hk device loin
func (device *HKDevice) LoginV4() (int, error) {
	// init data
	var deviceInfoV40 C.NET_DVR_DEVICEINFO_V40
	var userLoginInfo C.NET_DVR_USER_LOGIN_INFO
	// 使用strcpy函数将字符串复制到C结构体的字符数组中
	strcpy(unsafe.Pointer(&userLoginInfo.sDeviceAddress[0]), device.ip, len(userLoginInfo.sDeviceAddress))
	userLoginInfo.wPort = C.WORD(device.port)
	strcpy(unsafe.Pointer(&userLoginInfo.sUserName[0]), device.username, len(userLoginInfo.sUserName))
	strcpy(unsafe.Pointer(&userLoginInfo.sPassword[0]), device.password, len(userLoginInfo.sPassword))

	// 正确调用NET_DVR_Login_V40函数
	device.loginId = int(C.NET_DVR_Login_V40(&userLoginInfo, (*C.NET_DVR_DEVICEINFO_V40)(unsafe.Pointer(&deviceInfoV40))))
	// 正确地将_Ctype_BYTE数组转换为string
	serialNumber := string(unsafe.Slice(&deviceInfoV40.struDeviceV30.sSerialNumber[0], len(deviceInfoV40.struDeviceV30.sSerialNumber)))
	// 去除可能的空字符
	serialNumber = strings.Trim(serialNumber, "\x00")
	// fmt.Println("设备序列号:", serialNumber)
	// bySupportDev5是一个字节值而不是数组，直接输出其数值
	// fmt.Println("支持的设备类型标志:", int(deviceInfoV40.bySupportDev5))
	// fmt.Println("登录成功，设备信息已获取")
	if device.loginId < 0 {
		return -1, device.HKErr("login ")
	}
	log.Println("login success")
	return device.loginId, nil
}

func (device *HKDevice) GetDeiceInfo() (*DeviceInfo, error) {
	// BOOL NET_DVR_GetDVRConfig(LONG lUserID, DWORD dwCommand,LONG lChannel, LPVOID lpOutBuffer, DWORD dwOutBufferSize, LPDWORD lpBytesReturned);
	var deviceInfo C.NET_DVR_DEVICECFG
	var bytesReturned C.DWORD
	if C.NET_DVR_GetDVRConfig(C.LONG(device.loginId), C.DWORD(NET_DVR_GET_DEVICECFG), C.LONG(0), (C.LPVOID)(unsafe.Pointer(&deviceInfo)), C.DWORD(unsafe.Sizeof(deviceInfo)), &bytesReturned) != C.TRUE {
		// fmt.Println("获取设备信息失败")
	}
	sDVRName := string(unsafe.Slice(&deviceInfo.sDVRName[0], len(deviceInfo.sDVRName)))
	sSerialNumber := string(unsafe.Slice(&deviceInfo.sSerialNumber[0], len(deviceInfo.sSerialNumber)))

	// 清理字符串中的空字符和空格
	sDVRName = strings.TrimRight(sDVRName, "\x00")
	sDVRName = strings.TrimSpace(sDVRName)
	sSerialNumber = strings.TrimRight(sSerialNumber, "\x00")
	sSerialNumber = strings.TrimSpace(sSerialNumber)

	sDVRName, _ = GBKToUTF8([]byte(sDVRName))
	device.byChanNum = int(deviceInfo.byChanNum)
	return &DeviceInfo{
		IP:         device.ip,
		Port:       device.port,
		UserName:   device.username,
		Password:   device.password,
		DeviceID:   sSerialNumber,
		DeviceName: sDVRName,
		ByChanNum:  int(deviceInfo.byChanNum),
	}, nil
}

// 获取通道名称,俯仰角,横滚角
func (device *HKDevice) GetChannelName() (map[int]string, error) {
	channelNames := make(map[int]string)
	for i := 1; i <= int(device.byChanNum); i++ {
		var channelInfo C.NET_DVR_PICCFG
		var bytesReturned C.DWORD
		var sDVRName string
		// if C.NET_DVR_GetChannelInfo(C.LONG(device.loginId), C.LONG(i), (*C.NET_DVR_CHANNELINFO)(unsafe.Pointer(&channelInfo))) != C.TRUE {
		// 	return nil, device.HKErr("get channel info")
		// }
		if C.NET_DVR_GetDVRConfig(C.LONG(device.loginId), C.DWORD(NET_DVR_GET_PICCFG), C.LONG(i), (C.LPVOID)(unsafe.Pointer(&channelInfo)), C.DWORD(unsafe.Sizeof(channelInfo)), &bytesReturned) != C.TRUE {
			// fmt.Println("获取通道名称失败")
			// return nil, device.HKErr("get device info")
			sDVRName = "camera" + fmt.Sprintf("%d", i)
		} else {
			sDVRName = string(unsafe.Slice(&channelInfo.sChanName[0], len(channelInfo.sChanName)))
			// 清理字符串中的空字符和空格
			sDVRName = strings.TrimRight(sDVRName, "\x00")
			sDVRName = strings.TrimSpace(sDVRName)
			sDVRName, _ = GBKToUTF8([]byte(sDVRName))
		}
		channelNames[i] = sDVRName
	}
	return channelNames, nil
}

// 获取通道名称,俯仰角,横滚角
func (device *HKDevice) GetChannelPTZ(channel int) {
	var ptzPos C.NET_DVR_PTZPOS
	var ptzScope C.NET_DVR_PTZSCOPE
	var bytesReturned C.DWORD
	// if C.NET_DVR_GetChannelInfo(C.LONG(device.loginId), C.LONG(i), (*C.NET_DVR_CHANNELINFO)(unsafe.Pointer(&channelInfo))) != C.TRUE {
	// 	return nil, device.HKErr("get channel info")
	// }
	if C.NET_DVR_GetDVRConfig(C.LONG(device.loginId), C.DWORD(NET_DVR_GET_PTZPOS), C.LONG(channel), (C.LPVOID)(unsafe.Pointer(&ptzPos)), C.DWORD(unsafe.Sizeof(ptzPos)), &bytesReturned) != C.TRUE {
		// fmt.Println("获取PTZ位置信息失败")
	}
	// fmt.Println("PTZ位置信息:", ptzPos)

	if C.NET_DVR_GetDVRConfig(C.LONG(device.loginId), C.DWORD(NET_DVR_GET_PTZSCOPE), C.LONG(channel), (C.LPVOID)(unsafe.Pointer(&ptzScope)), C.DWORD(unsafe.Sizeof(ptzScope)), &bytesReturned) != C.TRUE {
		// fmt.Println("获取PTZ范围信息失败")
	}
	fmt.Println("PTZ范围信息:", ptzScope)
	// 计算PTZ位置 - 显示原始值用于调试
	fmt.Println("原始PTZ值 - wPanPos:", ptzPos.wPanPos, "wPanPosMin:", ptzScope.wPanPosMin, "wPanPosMax:", ptzScope.wPanPosMax)
	fmt.Println("原始PTZ值 - wTiltPos:", ptzPos.wTiltPos, "wTiltPosMin:", ptzScope.wTiltPosMin, "wTiltPosMax:", ptzScope.wTiltPosMax)
	fmt.Println("原始PTZ值 - wZoomPos:", ptzPos.wZoomPos, "wZoomPosMin:", ptzScope.wZoomPosMin, "wZoomPosMax:", ptzScope.wZoomPosMax)

	// 计算差值
	deltaPan := ptzScope.wPanPosMax - ptzScope.wPanPosMin
	deltaTilt := ptzScope.wTiltPosMax - ptzScope.wTiltPosMin
	deltaZoom := ptzScope.wZoomPosMax - ptzScope.wZoomPosMin

	fmt.Println("差值计算 - deltaPan:", deltaPan, "deltaTilt:", deltaTilt, "deltaZoom:", deltaZoom)
	fmt.Println("位置差值 - Pan:", ptzPos.wPanPos-ptzScope.wPanPosMin, "Tilt:", ptzPos.wTiltPos-ptzScope.wTiltPosMin, "Zoom:", ptzPos.wZoomPos-ptzScope.wZoomPosMin)

	// 添加除零检查和边界处理
	// 计算水平位置 (Pan)
	if deltaPan > 0 {
		// 计算实际比例
		panRatio := float64(ptzPos.wPanPos-ptzScope.wPanPosMin) / float64(deltaPan)
		fmt.Println("Pan比例:", panRatio)
		// 计算Pan位置（0-360度）
		panPos := int(panRatio * 360)
		// 确保结果在0-360范围内
		if panPos < 0 {
			panPos = 0
		} else if panPos > 360 {
			panPos = 360
		}
		ptzPos.wPanPos = C.WORD(panPos)
		fmt.Println("计算后Pan位置:", panPos)
	} else {
		ptzPos.wPanPos = 0 // 当范围无效时设置默认值
		fmt.Println("警告: PTZ水平范围无效，设置为默认值")
	}

	// 计算垂直位置 (Tilt)
	if deltaTilt > 0 {
		// 计算实际比例
		tiltRatio := float64(ptzPos.wTiltPos-ptzScope.wTiltPosMin) / float64(deltaTilt)
		fmt.Println("Tilt比例:", tiltRatio)
		// 计算Tilt位置（0-360度）
		tiltPos := int(tiltRatio * 90)
		// 确保结果在0-360范围内
		if tiltPos < 0 {
			tiltPos = 0
		} else if tiltPos > 90 {
			tiltPos = 90
		}
		ptzPos.wTiltPos = C.WORD(tiltPos)
		fmt.Println("计算后Tilt位置:", tiltPos)
	} else {
		ptzPos.wTiltPos = 0 // 当范围无效时设置默认值
		fmt.Println("警告: PTZ垂直范围无效，设置为默认值")
	}

	// 计算缩放位置 (Zoom)
	if deltaZoom > 0 {
		// 计算实际比例
		zoomRatio := float64(ptzPos.wZoomPos-ptzScope.wZoomPosMin) / float64(deltaZoom)
		fmt.Println("Zoom比例:", zoomRatio)
		// 计算Zoom位置（0-100%）
		zoomPos := int(zoomRatio * 100)
		// 确保结果在0-100范围内
		if zoomPos < 0 {
			zoomPos = 0
		} else if zoomPos > 100 {
			zoomPos = 100
		}
		ptzPos.wZoomPos = C.WORD(zoomPos)
		fmt.Println("计算后Zoom位置:", zoomPos)
	} else {
		ptzPos.wZoomPos = 0 // 当范围无效时设置默认值
		fmt.Println("警告: PTZ缩放范围无效，设置为默认值")
	}
	fmt.Println("PTZ位置信息:", ptzPos)

}

func (device *HKDevice) SetAlarmCallBack() error { //监听报警信息
	if C.NET_DVR_SetDVRMessageCallBack_V30(C.MSGCallBack(C.AlarmCallBack), C.NULL) != C.TRUE {
		return device.HKErr(device.ip + ":set alarm callback")
	}
	return nil
}
func (device *HKDevice) StartListenAlarmMsg() error {
	var struAlarmParam C.NET_DVR_SETUPALARM_PARAM
	// 根据平台使用正确的类型
	struAlarmParam.dwSize = C.DWORD(unsafe.Sizeof(struAlarmParam))
	struAlarmParam.byAlarmInfoType = 0

	// 转换为正确的类型
	device.alarmHandle = int(C.NET_DVR_SetupAlarmChan_V41(C.LONG(device.loginId), &struAlarmParam))
	if device.alarmHandle < 0 {
		return device.HKErr("setup alarm chan")
	}
	return nil
}

func (device *HKDevice) StopListenAlarmMsg() error {
	if C.NET_DVR_CloseAlarmChan_V30(C.LONG(device.alarmHandle)) != C.TRUE {
		return device.HKErr("stop alarm chan")
	}
	return nil
}

// HKErr Detect success of operation
func (device *HKDevice) HKErr(operation string) error {
	errno := int64(C.NET_DVR_GetLastError())
	if errno > 0 {
		reMsg := fmt.Sprintf("%s:%s摄像头失败,失败代码号：%d", device.ip, operation, errno)
		return errors.New(reMsg)
	}
	return nil
}

// // Login hk device loin
func (device *HKDevice) PTZControlWithSpeed(dwPTZCommand, dwStop, dwSpeed int) (bool, error) {
	// init data
	if C.NET_DVR_PTZControlWithSpeed(C.LONG(device.loginId), C.DWORD(dwPTZCommand), C.DWORD(dwStop), C.DWORD(dwSpeed)) != C.TRUE {
		return false, nil
	}

	log.Println("control success")
	return true, nil
}

// // Login hk device loin
func (device *HKDevice) PTZControlWithSpeed_Other(lChannel, dwPTZCommand, dwStop, dwSpeed int) (bool, error) {
	if C.NET_DVR_PTZControlWithSpeed_Other(C.LONG(device.loginId), C.LONG(lChannel), C.DWORD(dwPTZCommand), C.DWORD(dwStop), C.DWORD(dwSpeed)) != C.TRUE {
		return false, nil
	}

	log.Println("control success")
	return true, nil
}

// // Login hk device loin
func (device *HKDevice) PTZControl(dwPTZCommand, dwStop int) (bool, error) {
	// init data
	if C.NET_DVR_PTZControl(C.LONG(device.loginId), C.DWORD(dwPTZCommand), C.DWORD(dwStop)) != C.TRUE {
		return false, nil
	}

	log.Println("control success")
	return true, nil
}

// // Login hk device loin
func (device *HKDevice) PTZControl_Other(lChannel, dwPTZCommand, dwStop int) (bool, error) {
	// init data
	if C.NET_DVR_PTZControl_Other(C.LONG(device.loginId), C.LONG(lChannel), C.DWORD(dwPTZCommand), C.DWORD(dwStop)) != C.TRUE {
		return false, nil
	}

	log.Println("control success")
	return true, nil
}

func (device *HKDevice) RealPlay_V40(ChannelId int, receiver *Receiver) (int, error) {
	previewInfo := C.NET_DVR_PREVIEWINFO{}
	previewInfo.hPlayWnd = nil
	previewInfo.lChannel = C.LONG(ChannelId)
	previewInfo.dwStreamType = C.DWORD(0)
	previewInfo.dwLinkMode = C.DWORD(0)
	previewInfo.bBlocked = C.DWORD(0)
	previewInfo.byProtoType = C.BYTE(0)

	// 存储receiver指针并获取安全ID
	receiverID := storePointer(receiver)
	device.receiverID = receiverID

	//LONG NET_DVR_RealPlay_V40(LONG lUserID, LPNET_DVR_PREVIEWINFO lpPreviewInfo, REALDATACALLBACK fRealDataCallBack_V30, void* pUser);
	device.lRealHandle = int(C.NET_DVR_RealPlay_V40(C.LONG(device.loginId), &previewInfo, C.REALDATACALLBACK(C.RealDataCallBack_V30), unsafe.Pointer(receiverID)))
	log.Println("Play success", device.lRealHandle)
	return device.lRealHandle, nil
}

// StopRealPlay 停止预览
func (device *HKDevice) StopRealPlay() {
	if device.lRealHandle != 0 {
		C.NET_DVR_StopRealPlay(C.LONG(device.lRealHandle))
		device.lRealHandle = 0
		// 清理指针映射
		if device.receiverID != 0 {
			removePointer(device.receiverID)
			device.receiverID = 0
		}
		log.Println("Stop preview success")
	}
}
