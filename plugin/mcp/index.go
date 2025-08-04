package plugin_mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"m7s.live/v5"
)

type McpPlugin struct {
	m7s.Plugin
	mcpServer *server.SSEServer
}

var _ = m7s.InstallPlugin[McpPlugin](m7s.PluginMeta{})

// 基础 URL 常量
const (
	BaseURL = "http://localhost:8080"
)

func (p *McpPlugin) RegisterHandler() map[string]http.HandlerFunc {

	// 创建 MCP 服务器
	s := server.NewMCPServer(
		"Monibuca", // 服务器名称
		"1.0.0",    // 版本
		server.WithToolCapabilities(false),
	)

	// 添加系统信息 API
	sysInfoTool := mcp.NewTool(
		"getSysInfo",
		mcp.WithDescription("获取系统信息"),
	)
	s.AddTool(sysInfoTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		resp, err := http.Get(BaseURL + "/api/sysinfo")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("获取系统信息失败: %v", err)), nil
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("读取响应失败: %v", err)), nil
		}

		return mcp.NewToolResultText(string(body)), nil
	})

	// 添加服务器摘要 API
	summaryTool := mcp.NewTool(
		"getSummary",
		mcp.WithDescription("获取服务器摘要信息"),
	)
	s.AddTool(summaryTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		resp, err := http.Get(BaseURL + "/api/summary")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("获取服务器摘要失败: %v", err)), nil
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("读取响应失败: %v", err)), nil
		}

		return mcp.NewToolResultText(string(body)), nil
	})

	// 添加流列表 API
	streamListTool := mcp.NewTool(
		"getStreamList",
		mcp.WithDescription("获取流列表"),
	)
	s.AddTool(streamListTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		resp, err := http.Get(BaseURL + "/api/stream/list")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("获取流列表失败: %v", err)), nil
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("读取响应失败: %v", err)), nil
		}

		return mcp.NewToolResultText(string(body)), nil
	})

	// 添加流详情 API
	streamInfoTool := mcp.NewTool(
		"getStreamInfo",
		mcp.WithDescription("获取流详情"),
		mcp.WithString("streamPath",
			mcp.Required(),
			mcp.Description("流路径"),
		),
	)
	s.AddTool(streamInfoTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		streamPath, ok := request.Params.Arguments["streamPath"].(string)
		if !ok || streamPath == "" {
			return mcp.NewToolResultError("流路径不能为空"), nil
		}

		resp, err := http.Get(BaseURL + "/api/stream/info/" + streamPath)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("获取流详情失败: %v", err)), nil
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("读取响应失败: %v", err)), nil
		}

		return mcp.NewToolResultText(string(body)), nil
	})

	// 添加配置文件内容 API
	configFileTool := mcp.NewTool(
		"getConfigFile",
		mcp.WithDescription("获取配置文件内容"),
	)
	s.AddTool(configFileTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		resp, err := http.Get(BaseURL + "/api/config/file")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("获取配置文件内容失败: %v", err)), nil
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("读取响应失败: %v", err)), nil
		}

		return mcp.NewToolResultText(string(body)), nil
	})

	// 添加截图 API
	snapTool := mcp.NewTool(
		"getSnapshot",
		mcp.WithDescription("获取流的截图"),
		mcp.WithString("streamPath",
			mcp.Required(),
			mcp.Description("流路径"),
		),
		//mcp.WithBoolean("isUrl",
		//	mcp.Required(),
		//	mcp.Description("是否返回URL"),
		//	mcp.DefaultString("1"),
		//),
		//mcp.WithString("savePath",
		//	mcp.Required(),
		//	mcp.Description("保存路径"),
		//	mcp.DefaultString("snap"),
		//),
	)
	s.AddTool(snapTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		streamPath, ok := request.Params.Arguments["streamPath"].(string)
		if !ok || streamPath == "" {
			return mcp.NewToolResultError("流路径不能为空"), nil
		}

		isUrl := "1"
		//if val, ok := request.Params.Arguments["isUrl"].(bool); ok {
		//	isUrl = val
		//}

		savePath := "snap"
		//if val, ok := request.Params.Arguments["savePath"].(string); ok && val != "" {
		//	savePath = val
		//}

		url := fmt.Sprintf("%s/snap/%s?isUrl=%s&savePath=%s", BaseURL, streamPath, isUrl, savePath)
		resp, err := http.Get(url)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("获取截图失败: %v", err)), nil
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("读取响应失败: %v", err)), nil
		}

		// 解析JSON响应
		var result map[string]interface{}
		if err := json.Unmarshal(body, &result); err != nil {
			return mcp.NewToolResultText(string(body)), nil
		}

		// 构建响应
		var sb strings.Builder
		if url, ok := result["url"].(string); ok {
			sb.WriteString(fmt.Sprintf("截图URL: %s\n", url))
		}
		if markdown, ok := result["markdown"].(string); ok {
			sb.WriteString(fmt.Sprintf("Markdown: %s\n", markdown))
		}

		return mcp.NewToolResultText(sb.String()), nil
	})

	// 创建 SSE 服务器
	p.mcpServer = server.NewSSEServer(s,
		server.WithBaseURL(BaseURL),
		server.WithDynamicBasePath(func(r *http.Request, sessionID string) string {
			return "/mcp"
		}),
	)
	return map[string]http.HandlerFunc{
		"/sse": func(w http.ResponseWriter, r *http.Request) {
			p.mcpServer.SSEHandler().ServeHTTP(w, r)
		},
		"/message": func(w http.ResponseWriter, r *http.Request) {
			p.mcpServer.MessageHandler().ServeHTTP(w, r)
		},
	}
}

func (p *McpPlugin) Dispose() {
	// 关闭 MCP 服务器
	if p.mcpServer != nil {
		p.mcpServer.Shutdown(context.Background())
	}
}
