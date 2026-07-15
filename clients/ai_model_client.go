package clients

import (
	"ai-meeting/config"
	"ai-meeting/models"
	mysqlrepo "ai-meeting/repositories/mysql"
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"gorm.io/gorm"
)

type PromptMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatStreamChunk struct {
	Content          string
	ReasoningContent string
}

func CallConfiguredAIChat(ctx context.Context, aiID uint, messages []PromptMessage, temperature float64) (string, error) {
	prop, err := loadEnabledAiProperty(aiID)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(prop.Endpoint) == "" {
		return "", errors.New("enabled ai model has empty endpoint")
	}

	payload := map[string]interface{}{
		"model":       resolveAiModelName(prop),
		"temperature": temperature,
		"stream":      false,
		"messages":    messages,
	}

	req, err := newAIChatRequest(ctx, prop, payload)
	if err != nil {
		return "", err
	}

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("ai chat status %d: %s", resp.StatusCode, string(respBody))
	}

	return parseAIChatResponse(respBody)
}

func CallConfiguredAIChatStream(ctx context.Context, aiID uint, messages []PromptMessage, temperature float64, onChunk func(ChatStreamChunk) error) error {
	if onChunk == nil {
		return errors.New("ai chat stream callback is nil")
	}

	prop, err := loadEnabledAiProperty(aiID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(prop.Endpoint) == "" {
		return errors.New("enabled ai model has empty endpoint")
	}

	payload := map[string]interface{}{
		"model":       resolveAiModelName(prop),
		"temperature": temperature,
		"stream":      true,
		"messages":    messages,
	}

	req, err := newAIChatRequest(ctx, prop, payload)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		respBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			return readErr
		}
		return fmt.Errorf("ai chat stream status %d: %s", resp.StatusCode, string(respBody))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}
		if !strings.HasPrefix(line, "data:") {
			continue
		}

		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			return nil
		}

		chunk, err := parseAIChatStreamChunk([]byte(data))
		if err != nil {
			return err
		}
		if chunk.Content == "" && chunk.ReasoningContent == "" {
			continue
		}
		if err := onChunk(chunk); err != nil {
			return err
		}
	}

	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}

func newAIChatRequest(ctx context.Context, prop *models.AiProperties, payload map[string]interface{}) (*http.Request, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, prop.Endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if prop.ApiKey != "" {
		req.Header.Set("Authorization", "Bearer "+prop.ApiKey)
	}
	if prop.ApiSecret != "" {
		req.Header.Set("X-API-Secret", prop.ApiSecret)
	}
	return req, nil
}

func loadEnabledAiProperty(aiID uint) (*models.AiProperties, error) {
	if aiID == 0 {
		if prop, ok := loadConfiguredDeepSeekProperty(); ok {
			return prop, nil
		}
	}

	prop, err := mysqlrepo.FindEnabledAiProperty(aiID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if prop, ok := loadConfiguredDeepSeekProperty(); ok {
				return prop, nil
			}
			if aiID > 0 {
				return nil, errors.New("selected ai model is not enabled or does not exist")
			}
			return nil, errors.New("no enabled ai model configured")
		}
		return nil, err
	}
	return prop, nil
}

func loadConfiguredDeepSeekProperty() (*models.AiProperties, bool) {
	aiConfig := config.AppConfig.AI
	deepSeek := aiConfig.DeepSeek
	if provider := strings.TrimSpace(aiConfig.Provider); provider != "" && !strings.EqualFold(provider, "deepseek") {
		return nil, false
	}
	if !deepSeek.Enabled || strings.TrimSpace(deepSeek.APIKey) == "" {
		return nil, false
	}

	endpoint := strings.TrimSpace(deepSeek.Endpoint)
	if endpoint == "" {
		endpoint = "https://api.deepseek.com/chat/completions"
	}

	modelName := strings.TrimSpace(deepSeek.Model)
	if modelName == "" {
		modelName = "deepseek-chat"
	}

	return &models.AiProperties{
		Name:      "deepseek",
		ModelType: modelName,
		ApiKey:    strings.TrimSpace(deepSeek.APIKey),
		ApiSecret: strings.TrimSpace(deepSeek.APISecret),
		Endpoint:  endpoint,
		IsEnabled: true,
	}, true
}

func parseAIChatResponse(respBody []byte) (string, error) {
	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			Text string `json:"text"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", err
	}
	if len(result.Choices) == 0 {
		return "", errors.New("ai chat response has no choices")
	}
	if content := strings.TrimSpace(result.Choices[0].Message.Content); content != "" {
		return content, nil
	}
	if text := strings.TrimSpace(result.Choices[0].Text); text != "" {
		return text, nil
	}
	return "", errors.New("ai chat response content is empty")
}

func parseAIChatStreamChunk(data []byte) (ChatStreamChunk, error) {
	var result struct {
		Choices []struct {
			Delta struct {
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
			} `json:"delta"`
			Text string `json:"text"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return ChatStreamChunk{}, err
	}
	if len(result.Choices) == 0 {
		return ChatStreamChunk{}, nil
	}

	chunk := ChatStreamChunk{
		Content:          result.Choices[0].Delta.Content,
		ReasoningContent: result.Choices[0].Delta.ReasoningContent,
	}
	if chunk.Content == "" && result.Choices[0].Text != "" {
		chunk.Content = result.Choices[0].Text
	}
	return chunk, nil
}

func resolveAiModelName(prop *models.AiProperties) string {
	modelName := strings.TrimSpace(prop.ModelType)
	if prop.Config != "" {
		var cfg map[string]interface{}
		if err := json.Unmarshal([]byte(prop.Config), &cfg); err == nil {
			if val, ok := cfg["model"].(string); ok && strings.TrimSpace(val) != "" {
				modelName = strings.TrimSpace(val)
			}
		}
	}
	if modelName == "" {
		modelName = prop.Name
	}
	return modelName
}
