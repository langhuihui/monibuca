package plugin_gb28181pro

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	gb28181 "m7s.live/v5/plugin/gb28181/pkg"
)

// handleDownloadFile 处理文件下载请求
// URL: /gb28181/download?downloadId=xxx
func (gb *GB28181Plugin) handleDownloadFile(w http.ResponseWriter, r *http.Request) {
	// 获取 downloadId 参数
	downloadId := r.URL.Query().Get("downloadId")
	if downloadId == "" {
		http.Error(w, "downloadId parameter is required", http.StatusBadRequest)
		return
	}

	// 检查数据库
	if gb.DB == nil {
		http.Error(w, "Database not available", http.StatusInternalServerError)
		gb.Error("数据库未初始化")
		return
	}

	// 从 gb28181_record 表查询文件路径
	var record gb28181.GB28181Record
	if err := gb.DB.Where("download_id = ? AND status = ?", downloadId, "completed").First(&record).Error; err != nil {
		// 检查是否是正在进行的下载任务
		if dialog, exists := gb.downloadDialogs.Get(downloadId); exists {
			http.Error(w, "Download in progress", http.StatusAccepted)
			gb.Info("下载任务进行中",
				"downloadId", downloadId,
				"status", dialog.Status,
				"progress", dialog.Progress)
			return
		}

		http.Error(w, "Download record not found or not completed", http.StatusNotFound)
		gb.Warn("下载记录不存在或未完成",
			"downloadId", downloadId,
			"error", err)
		return
	}

	filePath, _ := gb.Server.Storage.GetURL(gb, record.FilePath)
	filename := filepath.Base(filePath)

	gb.Info("从缓存记录获取文件路径",
		"downloadId", downloadId,
		"filePath", filePath)

	// 检查文件是否存在
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "File not found", http.StatusNotFound)
		} else {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		gb.Error("文件访问失败", "filePath", filePath, "error", err)
		return
	}

	// 检查是否是文件（不是目录）
	if fileInfo.IsDir() {
		http.Error(w, "Path is a directory", http.StatusBadRequest)
		return
	}

	// 打开文件
	file, err := os.Open(filePath)
	if err != nil {
		http.Error(w, "Failed to open file", http.StatusInternalServerError)
		gb.Error("打开文件失败", "filePath", filePath, "error", err)
		return
	}
	defer file.Close()

	// 设置响应头
	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	w.Header().Set("Content-Length", strconv.FormatInt(fileInfo.Size(), 10))
	w.Header().Set("Accept-Ranges", "bytes")

	// 支持断点续传
	http.ServeContent(w, r, filename, fileInfo.ModTime(), file)

	gb.Info("文件下载",
		"filename", filename,
		"filePath", filePath,
		"size", fileInfo.Size(),
		"remote", r.RemoteAddr)
}
