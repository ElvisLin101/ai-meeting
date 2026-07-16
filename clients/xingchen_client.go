package clients

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	xingChenChatURL     = "https://xingchen-api.xf-yun.com/workflow/v1/chat/completions"
	xingChenUploadURL   = "https://xingchen-api.xf-yun.com/workflow/v1/upload_file"
	xingChenDefaultUID  = "123"
	xingChenHTTPTimeout = 5 * time.Minute
)

// XingChenChatRequest 讯飞星辰工作流请求体
type XingChenChatRequest struct {
	FlowID     string                 `json:"flow_id"`
	UID        string                 `json:"uid"`
	Stream     bool                   `json:"stream"`
	ChatID     string                 `json:"chat_id"`
	History    []XingChenHistoryItem  `json:"history"`
	Parameters map[string]interface{} `json:"parameters"`
}

// XingChenHistoryItem 历史对话条目
type XingChenHistoryItem struct {
	Role        string `json:"role"`
	ContentType string `json:"content_type"`
	Content     string `json:"content"`
}

// XingChenClient 讯飞星辰工作流客户端
type XingChenClient struct{}

var xingChenClientInstance *XingChenClient

func GetXingChenClient() *XingChenClient {
	if xingChenClientInstance == nil {
		xingChenClientInstance = &XingChenClient{}
	}
	return xingChenClientInstance
}

// ChatStream 流式调用讯飞星辰工作流
// onChunk 回调用于实时推送每个 SSE chunk 给前端
// 返回累积的完整内容
func (c *XingChenClient) ChatStream(
	input, chatID string,
	history []XingChenHistoryItem,
	flowID, apiKey, apiSecret string,
	onChunk func(chunk string),
) (string, error) {
	return c.ChatStreamWithFile(input, chatID, history, flowID, apiKey, apiSecret, "", nil, onChunk)
}

// ChatStreamWithFile 流式调用（支持文件 URL 和额外参数）
func (c *XingChenClient) ChatStreamWithFile(
	input, chatID string,
	history []XingChenHistoryItem,
	flowID, apiKey, apiSecret, fileURL string,
	extraParameters map[string]interface{},
	onChunk func(chunk string),
) (string, error) {
	parameters := buildXingChenParameters(input, fileURL, extraParameters)

	reqBody := XingChenChatRequest{
		FlowID:     flowID,
		UID:        xingChenDefaultUID,
		Stream:     true,
		ChatID:     chatID,
		History:    history,
		Parameters: parameters,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("序列化请求体失败: %w", err)
	}

	req, err := http.NewRequest("POST", xingChenChatURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("构建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+apiKey+":"+apiSecret)

	client := &http.Client{Timeout: xingChenHTTPTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求讯飞星辰失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		errBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("讯飞星辰返回错误状态码 %d: %s", resp.StatusCode, string(errBody))
	}

	var contentBuilder strings.Builder
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		payload := extractSsePayload(line)
		if payload == "" {
			continue
		}

		if onChunk != nil {
			onChunk(payload)
		}

		content := parseContentFromChunk(payload)
		if content != "" {
			contentBuilder.WriteString(content)
		}
	}

	if err := scanner.Err(); err != nil {
		return contentBuilder.String(), fmt.Errorf("读取SSE流失败: %w", err)
	}

	return contentBuilder.String(), nil
}

// ChatSync 同步调用讯飞星辰工作流，返回完整响应字符串
func (c *XingChenClient) ChatSync(
	input, chatID string,
	history []XingChenHistoryItem,
	flowID, apiKey, apiSecret string,
	extraParameters map[string]interface{},
) (string, error) {
	return c.ChatSyncWithFile(input, chatID, history, flowID, apiKey, apiSecret, "", extraParameters)
}

// ChatSyncWithFile 同步调用（支持文件 URL）
func (c *XingChenClient) ChatSyncWithFile(
	input, chatID string,
	history []XingChenHistoryItem,
	flowID, apiKey, apiSecret, fileURL string,
	extraParameters map[string]interface{},
) (string, error) {
	parameters := buildXingChenParameters(input, fileURL, extraParameters)

	reqBody := XingChenChatRequest{
		FlowID:     flowID,
		UID:        xingChenDefaultUID,
		Stream:     false,
		ChatID:     chatID,
		History:    history,
		Parameters: parameters,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("序列化请求体失败: %w", err)
	}

	req, err := http.NewRequest("POST", xingChenChatURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("构建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey+":"+apiSecret)

	client := &http.Client{Timeout: xingChenHTTPTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求讯飞星辰失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("讯飞星辰返回错误状态码 %d: %s", resp.StatusCode, string(body))
	}

	logrus.Infof("XingChen sync response: %s", string(body))
	return string(body), nil
}

// UploadFile 上传文件到讯飞星辰平台，返回文件 URL
func (c *XingChenClient) UploadFile(filePath, filename, apiKey, apiSecret string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("打开文件失败: %w", err)
	}
	defer file.Close()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return "", fmt.Errorf("创建 form file 失败: %w", err)
	}
	if _, err = io.Copy(part, file); err != nil {
		return "", fmt.Errorf("写入文件内容失败: %w", err)
	}

	if err = writer.Close(); err != nil {
		return "", fmt.Errorf("关闭 multipart writer 失败: %w", err)
	}

	req, err := http.NewRequest("POST", xingChenUploadURL, body)
	if err != nil {
		return "", fmt.Errorf("构建请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey+":"+apiSecret)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("上传文件失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("上传文件失败状态码 %d: %s", resp.StatusCode, string(respBody))
	}

	var uploadResp struct {
		Code int    `json:"code"`
		Msg  string `json:"message"`
		Data struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBody, &uploadResp); err != nil {
		return "", fmt.Errorf("解析上传响应失败: %w, body=%s", err, string(respBody))
	}
	if uploadResp.Code != 0 {
		return "", fmt.Errorf("上传文件失败 code=%d message=%s", uploadResp.Code, uploadResp.Msg)
	}
	if uploadResp.Data.URL == "" {
		return "", fmt.Errorf("上传响应中 url 为空: %s", string(respBody))
	}

	logrus.Infof("XingChen file uploaded: %s", uploadResp.Data.URL)
	return uploadResp.Data.URL, nil
}

// buildXingChenParameters 构造工作流参数
func buildXingChenParameters(input string, fileURL string, extraParameters map[string]interface{}) map[string]interface{} {
	parameters := map[string]interface{}{
		"AGENT_USER_INPUT": input,
	}
	for k, v := range extraParameters {
		if v != nil {
			parameters[k] = v
		}
	}
	if fileURL != "" {
		parameters["USER_FILE"] = fileURL
	}
	return parameters
}

// extractSsePayload 从 SSE 行中提取有效 payload
func extractSsePayload(line string) string {
	if strings.HasPrefix(line, "data: ") {
		data := strings.TrimSpace(line[6:])
		if strings.HasPrefix(data, "{") || data == "[DONE]" {
			return data
		}
		return ""
	}
	if strings.HasPrefix(line, "{") || line == "[DONE]" {
		return line
	}
	return ""
}

// parseContentFromChunk 从 SSE chunk JSON 中提取 content 字段
func parseContentFromChunk(payload string) string {
	if payload == "" || payload == "[DONE]" {
		return ""
	}

	var chunk struct {
		Choices []struct {
			Delta struct {
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
			} `json:"delta"`
		} `json:"choices"`
		Data struct {
			Content string `json:"content"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
		return ""
	}

	if len(chunk.Choices) > 0 {
		return chunk.Choices[0].Delta.Content
	}
	return chunk.Data.Content
}
