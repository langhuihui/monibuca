package plugin_mp4

import (
	"bytes"
	"encoding/json"
	"github.com/shirou/gopsutil/v4/disk"
	"m7s.live/v5"
	"net/http"
	"os"
	"time"
)

// mysql数据库里Exception 定义异常结构体
type Exception struct {
	CreateTime string `json:"createTime" gorm:"type:varchar(50)"`
	AlarmType  string `json:"alarmType" gorm:"type:varchar(50)"`
	AlarmDesc  string `json:"alarmDesc" gorm:"type:varchar(50)"`
	ServerIP   string `json:"serverIP" gorm:"type:varchar(50)"`
	StreamPath string `json:"streamPath" gorm:"type:varchar(50)"`
}

// 向第三方发送异常报警
func (p *MP4Plugin) SendToThirdPartyAPI(exception *Exception) {
	exception.CreateTime = time.Now().Format("2006-01-02 15:04:05")
	exception.ServerIP = p.GetCommonConf().PublicIP
	data, err := json.Marshal(exception)
	if err != nil {
		p.Error("SendToThirdPartyAPI", " marshalling exception error", err.Error())
		return
	}
	err = p.DB.Create(&exception).Error
	if err != nil {
		p.Error("SendToThirdPartyAPI", "insert into db error", err.Error())
		return
	}
	resp, err := http.Post(MP4Plugin{}.ExceptionPostUrl, "application/json", bytes.NewBuffer(data))
	if err != nil {
		p.Error("SendToThirdPartyAPI", "Error sending exception to third party API error", err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		p.Error("SendToThirdPartyAPI", "Failed to send exception, status code:", resp.StatusCode)
	} else {
		p.Info("SendToThirdPartyAPI", "Exception sent successfully!")
	}
}

// 磁盘超上限报警
func (p *MP4Plugin) getDisckException(streamPath string) bool {
	if p.getDiskOutOfSpace(p.DiskMaxPercent) {
		exceptionChannel <- &Exception{AlarmType: "disk alarm", AlarmDesc: "disk is full", StreamPath: streamPath}
		return true
	}
	return false
}

// 判断磁盘使用量是否中超限
func (p *MP4Plugin) getDiskOutOfSpace(max float64) bool {
	exePath, err := os.Getwd()
	//pwd, _ := os.Getwd()
	//fmt.Printf("当前pwd是: %v\n", pwd)
	//if err != nil {
	//	fmt.Printf("Error getting executable path: %v\n", err)
	//	return false
	//}
	//// 获取路径的根目录部分
	//root := filepath.VolumeName(exePath)
	//if root == "" {
	//	// 在Unix-like系统中，根目录是 "/"
	//	root = "/"
	//}
	d, err := disk.Usage(exePath)
	if err != nil {
		p.Error("getDiskOutOfSpace======get disk usage error:", err.Error())
	}
	p.Info("getDiskOutOfSpace", "current path", exePath, "disk UsedPercent", d.UsedPercent, "total disk space", d.Total,
		"disk free", d.Free, "disk usage", d.Used, "AutoOverWriteDiskPercent", p.AutoOverWriteDiskPercent, "DiskMaxPercent", p.DiskMaxPercent)
	if d.UsedPercent >= max {
		return true
	} else {
		return false
	}
}

func (p *MP4Plugin) deleteOldestFile() {
	//当当前磁盘使用量大于AutoOverWriteDiskPercent自动覆盖磁盘使用量配置时，自动删除最旧的文件
	//连续录像删除最旧的文件
	if p.getDiskOutOfSpace(p.AutoOverWriteDiskPercent) {
		queryRecord := m7s.RecordStream{
			EventLevel: "1", // 查询条件：event_level = 1,非重要事件
		}
		var eventRecords []m7s.RecordStream
		err := p.DB.Where(&queryRecord).Order("end_time ASC").Limit(1).Find(&eventRecords).Error
		if err == nil {
			if len(eventRecords) > 0 {
				for _, record := range eventRecords {
					p.Info("deleteOldestFile", "ready to delete oldestfile,ID", record.ID, "create time", record.EndTime, "filepath", record.FilePath)
					err = os.Remove(record.FilePath)
					if err != nil {
						p.Error("deleteOldestFile", "delete file from disk error", err.Error())
					}
					err = p.DB.Delete(&record).Error
					if err != nil {
						p.Error("deleteOldestFile", "delete record from disk error", err.Error())
					}
				}
			}
		} else {
			p.Error("deleteOldestFile", "search record from db error", err.Error())
		}
	}
}
