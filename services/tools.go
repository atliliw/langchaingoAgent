package services

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type WeatherInput struct {
	City string `json:"city"`
}

type WeatherOutput struct {
	City    string `json:"city"`
	Weather string `json:"weather"`
	Raw     string `json:"raw"`
}

type WeatherTool struct{}

func NewWeatherTool() *WeatherTool {
	return &WeatherTool{}
}

func (t *WeatherTool) Name() string {
	return "weather"
}

func (t *WeatherTool) Description() string {
	return "查询指定城市的当前天气"
}

func (t *WeatherTool) Invoke(input WeatherInput) (*WeatherOutput, error) {
	cityEncoded := urlEncode(input.City)
	url := fmt.Sprintf("https://wttr.in/%s?format=%s&lang=zh", cityEncoded, urlEncode("%C+|+%t+|+%h+|+%w"))

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("天气请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取天气响应失败: %w", err)
	}

	text := strings.TrimSpace(string(body))
	return &WeatherOutput{
		City:    input.City,
		Weather: text,
		Raw:     text,
	}, nil
}

func urlEncode(s string) string {
	var result strings.Builder
	for _, r := range s {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' || r == '~' {
			result.WriteRune(r)
		} else {
			result.WriteString(fmt.Sprintf("%%%02X", r))
		}
	}
	return result.String()
}
