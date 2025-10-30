package plugin_gb28181pro

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// handleDownloadFile 处理文件下载请求
func (gb *GB28181Plugin) handleDownloadFile(w http.ResponseWriter, r *http.Request) {
	// 从 URL 路径中提取参数
	// 路径格式：/gb28181/download/{deviceId}/{channelId}/{filename}
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(pathParts) < 3 {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	deviceId := pathParts[len(pathParts)-3]
	channelId := pathParts[len(pathParts)-2]
	filename := pathParts[len(pathParts)-1]

	// 验证文件名格式（防止路径遍历攻击）
	if strings.Contains(filename, "..") || strings.Contains(filename, "/") {
		http.Error(w, "Invalid filename", http.StatusBadRequest)
		return
	}

	// 构建文件路径
	filePath := filepath.Join("download", deviceId, channelId, filename)

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
		"deviceId", deviceId,
		"channelId", channelId,
		"filename", filename,
		"size", fileInfo.Size(),
		"remote", r.RemoteAddr)
}
