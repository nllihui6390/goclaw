package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// WeatherResult 天气查询结果（JSON格式）
type WeatherResult struct {
	Location    string        `json:"location"`
	Temperature string        `json:"temperature"`
	FeelsLike   string        `json:"feels_like"`
	Weather     string        `json:"weather"`
	WindDir     string        `json:"wind_dir"`
	WindSpeed   string        `json:"wind_speed"`
	Humidity    string        `json:"humidity"`
	Pressure    string        `json:"pressure"`
	Visibility  string        `json:"visibility"`
	Forecast    []DayForecast `json:"forecast,omitempty"`
}

// DayForecast 每日天气预报
type DayForecast struct {
	Date    string `json:"date"`
	MinTemp string `json:"min_temp"`
	MaxTemp string `json:"max_temp"`
	Weather string `json:"weather"`
}

// WeatherTool 天气查询工具（真实API版本）
type WeatherTool struct {
	apiKey     string // API密钥
	apiType    string // 使用的API类型: "hefeng", "openweather", "seniverse"
	httpClient *http.Client
}

// WeatherAPIConfig 天气API配置
type WeatherAPIConfig struct {
	Type    string // hefeng, openweather, seniverse
	APIKey  string
	BaseURL string
}

// NewWeatherTool 创建天气工具（使用默认配置）
func NewWeatherTool() *WeatherTool {
	return &WeatherTool{
		apiKey:  "", // 留空则使用免费API
		apiType: "hefeng",
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// NewWeatherToolWithConfig 使用配置创建天气工具
func NewWeatherToolWithConfig(cfg WeatherAPIConfig) *WeatherTool {
	if cfg.APIKey == "" {
		// 使用免费的演示API Key（仅供测试，生产环境请自行注册）
		cfg.APIKey = "YOUR_API_KEY"
	}

	return &WeatherTool{
		apiKey:  cfg.APIKey,
		apiType: cfg.Type,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (t *WeatherTool) Name() string {
	return "get_weather"
}

func (t *WeatherTool) Description() string {
	return "获取指定城市的实时天气信息和天气预报。支持中国城市和国外主要城市。"
}

func (t *WeatherTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"city": map[string]interface{}{
				"type":        "string",
				"description": "城市名称，支持中文或英文，如：北京、上海、New York、London",
			},
		},
		"required": []string{"city"},
	}
}

func (t *WeatherTool) Execute(ctx context.Context, params map[string]interface{}) (string, error) {
	city, ok := params["city"].(string)
	if !ok {
		return "", fmt.Errorf("缺少城市参数")
	}

	// 调用真实API获取天气
	result, err := t.fetchWeather(ctx, city)
	if err != nil {
		return "", fmt.Errorf("天气查询失败: %w", err)
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("JSON序列化失败: %w", err)
	}

	return string(jsonBytes), nil
}

// fetchWeather 调用真实天气API
func (t *WeatherTool) fetchWeather(ctx context.Context, city string) (*WeatherResult, error) {
	// 首先尝试配置的API
	switch t.apiType {
	case "openweather":
		result, err := t.fetchOpenWeather(ctx, city)
		if err == nil {
			return result, nil
		}
	case "seniverse":
		result, err := t.fetchSeniverse(ctx, city)
		if err == nil {
			return result, nil
		}
	default:
		result, err := t.fetchHefeng(ctx, city)
		if err == nil {
			return result, nil
		}
	}

	// API失败时，使用 wttr.in 作为后备（无需API key）
	return t.fetchWttr(ctx, city)
}

// fetchWttr 使用 wttr.in 获取天气（免费，无需API key）
func (t *WeatherTool) fetchWttr(ctx context.Context, city string) (*WeatherResult, error) {
	// wttr.in 支持中文城市名，format=j1 返回JSON
	url := fmt.Sprintf("https://wttr.in/%s?format=j1", url.QueryEscape(city))

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	// 设置中文语言
	req.Header.Set("Accept-Language", "zh-CN")

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("wttr.in请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var data struct {
		CurrentCondition []struct {
			TempC          string `json:"temp_C"`
			FeelsLikeC     string `json:"FeelsLikeC"`
			Humidity       string `json:"humidity"`
			WindDir16Point string `json:"winddir16Point"`
			WindSpeedKmph  string `json:"windspeedKmph"`
			WeatherDesc    []struct {
				Value string `json:"value"`
			} `json:"weatherDesc"`
		} `json:"current_condition"`
		Weather []struct {
			Date      string `json:"date"`
			MaxTempC  string `json:"maxtempC"`
			MinTempC  string `json:"mintempC"`
			WeatherDesc []struct {
				Value string `json:"value"`
			} `json:"weatherDesc"`
		} `json:"weather"`
		NearestArea []struct {
			AreaName []struct {
				Value string `json:"value"`
			} `json:"areaName"`
		} `json:"nearest_area"`
	}

	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("wttr.in响应解析失败: %w", err)
	}

	if len(data.CurrentCondition) == 0 {
		return nil, fmt.Errorf("未找到天气数据")
	}

	current := data.CurrentCondition[0]
	location := city
	if len(data.NearestArea) > 0 && len(data.NearestArea[0].AreaName) > 0 {
		location = data.NearestArea[0].AreaName[0].Value
	}

	weatherDesc := "未知"
	if len(current.WeatherDesc) > 0 {
		weatherDesc = current.WeatherDesc[0].Value
	}

	result := &WeatherResult{
		Location:    location,
		Temperature: current.TempC + "°C",
		FeelsLike:   current.FeelsLikeC + "°C",
		Weather:     weatherDesc,
		WindDir:     current.WindDir16Point,
		WindSpeed:   current.WindSpeedKmph + " km/h",
		Humidity:    current.Humidity + "%",
	}

	// 添加未来2天预报
	if len(data.Weather) >= 3 {
		forecasts := make([]DayForecast, 0)
		for i := 1; i < 3 && i < len(data.Weather); i++ {
			w := data.Weather[i]
			desc := "未知"
			if len(w.WeatherDesc) > 0 {
				desc = w.WeatherDesc[0].Value
			}
			forecasts = append(forecasts, DayForecast{
				Date:    w.Date,
				MinTemp: w.MinTempC + "°C",
				MaxTemp: w.MaxTempC + "°C",
				Weather: desc,
			})
		}
		result.Forecast = forecasts
	}

	return result, nil
}

// fetchHefeng 和风天气API（推荐，对中国城市支持好）
func (t *WeatherTool) fetchHefeng(ctx context.Context, city string) (*WeatherResult, error) {
	// 和风天气API端点
	// 注意：需要注册获取免费API Key: https://dev.qweather.com/
	// 免费版每天1000次调用，足够测试使用

	apiKey := t.apiKey
	if apiKey == "" {
		// 使用公开的演示Key（仅用于测试，有调用限制）
		apiKey = "YOUR_HEFENG_KEY" // 请替换为你的实际Key
	}

	// 1. 先获取城市ID
	cityURL := fmt.Sprintf("https://geoapi.qweather.com/v2/city/lookup?location=%s&key=%s",
		url.QueryEscape(city), apiKey)

	cityReq, err := http.NewRequestWithContext(ctx, "GET", cityURL, nil)
	if err != nil {
		return nil, err
	}

	cityResp, err := t.httpClient.Do(cityReq)
	if err != nil {
		return nil, err
	}
	defer cityResp.Body.Close()

	var cityData struct {
		Code     string `json:"code"`
		Location []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			Adm1 string `json:"adm1"`
			Adm2 string `json:"adm2"`
		} `json:"location"`
	}

	body, _ := io.ReadAll(cityResp.Body)
	if err := json.Unmarshal(body, &cityData); err != nil {
		return nil, err
	}

	if cityData.Code != "200" || len(cityData.Location) == 0 {
		return nil, fmt.Errorf("未找到城市: %s", city)
	}

	cityID := cityData.Location[0].ID
	cityName := cityData.Location[0].Name
	province := cityData.Location[0].Adm1

	// 2. 获取实时天气
	weatherURL := fmt.Sprintf("https://devapi.qweather.com/v7/weather/now?location=%s&key=%s",
		cityID, apiKey)

	weatherReq, _ := http.NewRequestWithContext(ctx, "GET", weatherURL, nil)
	weatherResp, err := t.httpClient.Do(weatherReq)
	if err != nil {
		return nil, err
	}
	defer weatherResp.Body.Close()

	var weatherData struct {
		Code string `json:"code"`
		Now  struct {
			Temp      string `json:"temp"`
			FeelsLike string `json:"feelsLike"`
			Text      string `json:"text"`
			WindDir   string `json:"windDir"`
			WindScale string `json:"windScale"`
			Humidity  string `json:"humidity"`
			Pressure  string `json:"pressure"`
			Vis       string `json:"vis"`
		} `json:"now"`
	}

	body, _ = io.ReadAll(weatherResp.Body)
	if err := json.Unmarshal(body, &weatherData); err != nil {
		return nil, err
	}

	if weatherData.Code != "200" {
		return nil, fmt.Errorf("获取天气失败: %s", weatherData.Code)
	}

	// 构建结果
	location := cityName
	if province != "" && province != cityName {
		location = province + " " + cityName
	}

	result := &WeatherResult{
		Location:    location,
		Temperature: weatherData.Now.Temp + "°C",
		FeelsLike:   weatherData.Now.FeelsLike + "°C",
		Weather:     weatherData.Now.Text,
		WindDir:     weatherData.Now.WindDir,
		WindSpeed:   weatherData.Now.WindScale + "级",
		Humidity:    weatherData.Now.Humidity + "%",
		Pressure:    weatherData.Now.Pressure + " hPa",
		Visibility:  weatherData.Now.Vis + " km",
	}

	return result, nil
}

// fetchOpenWeather OpenWeatherMap API
func (t *WeatherTool) fetchOpenWeather(ctx context.Context, city string) (*WeatherResult, error) {
	// OpenWeatherMap API (需要注册)
	// https://openweathermap.org/api

	apiKey := t.apiKey
	if apiKey == "" {
		apiKey = "YOUR_OPENWEATHER_KEY" // 请替换
	}

	// 使用公用的演示Key（仅供测试）
	if apiKey == "YOUR_OPENWEATHER_KEY" {
		apiKey = "bd5e378503939ddaee76f12ad7a97608" // 公开的演示Key
	}

	url := fmt.Sprintf("https://api.openweathermap.org/data/2.5/weather?q=%s&appid=%s&units=metric&lang=zh_cn",
		url.QueryEscape(city), apiKey)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data struct {
		Name string `json:"name"`
		Sys  struct {
			Country string `json:"country"`
		} `json:"sys"`
		Main struct {
			Temp      float64 `json:"temp"`
			FeelsLike float64 `json:"feels_like"`
			Humidity  int     `json:"humidity"`
			Pressure  int     `json:"pressure"`
		} `json:"main"`
		Weather []struct {
			Description string `json:"description"`
			Main        string `json:"main"`
		} `json:"weather"`
		Wind struct {
			Speed float64 `json:"speed"`
			Deg   int     `json:"deg"`
		} `json:"wind"`
		Visibility int `json:"visibility"`
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}

	if data.Name == "" {
		return nil, fmt.Errorf("未找到城市: %s", city)
	}

	// 风向度数转文字
	windDir := getWindDirection(data.Wind.Deg)

	// 能见度转换为km
	visibility := float64(data.Visibility) / 1000

	result := &WeatherResult{
		Location:    fmt.Sprintf("%s, %s", data.Name, data.Sys.Country),
		Temperature: fmt.Sprintf("%.1f°C", data.Main.Temp),
		FeelsLike:   fmt.Sprintf("%.1f°C", data.Main.FeelsLike),
		Weather:     fmt.Sprintf("%s - %s", data.Weather[0].Main, data.Weather[0].Description),
		WindDir:     windDir,
		WindSpeed:   fmt.Sprintf("%.1f m/s", data.Wind.Speed),
		Humidity:    fmt.Sprintf("%d%%", data.Main.Humidity),
		Pressure:    fmt.Sprintf("%d hPa", data.Main.Pressure),
		Visibility:  fmt.Sprintf("%.1f km", visibility),
	}

	return result, nil
}

// fetchSeniverse 心知天气API（中文友好）
func (t *WeatherTool) fetchSeniverse(ctx context.Context, city string) (*WeatherResult, error) {
	// 心知天气API (需要注册)
	// https://www.seniverse.com/

	apiKey := t.apiKey
	if apiKey == "" {
		apiKey = "YOUR_SENIVERSE_KEY" // 请替换
	}

	url := fmt.Sprintf("https://api.seniverse.com/v3/weather/now.json?key=%s&location=%s&language=zh-Hans&unit=c",
		apiKey, url.QueryEscape(city))

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var data struct {
		Results []struct {
			Location struct {
				Name string `json:"name"`
				Path string `json:"path"`
			} `json:"location"`
			Now struct {
				Text        string `json:"text"`
				Code        string `json:"code"`
				Temperature string `json:"temperature"`
				FeelsLike   string `json:"feels_like"`
				WindDir     string `json:"wind_direction"`
				WindScale   string `json:"wind_scale"`
				Humidity    string `json:"humidity"`
				Pressure    string `json:"pressure"`
				Visibility  string `json:"visibility"`
			} `json:"now"`
		} `json:"results"`
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if err := json.Unmarshal(body, &data); err != nil {
		return nil, err
	}

	if len(data.Results) == 0 {
		return nil, fmt.Errorf("未找到城市: %s", city)
	}

	r := data.Results[0]

	result := &WeatherResult{
		Location:    fmt.Sprintf("%s (%s)", r.Location.Name, r.Location.Path),
		Temperature: r.Now.Temperature + "°C",
		FeelsLike:   r.Now.FeelsLike + "°C",
		Weather:     r.Now.Text,
		WindDir:     r.Now.WindDir,
		WindSpeed:   r.Now.WindScale + "级",
		Humidity:    r.Now.Humidity + "%",
		Pressure:    r.Now.Pressure + " hPa",
		Visibility:  r.Now.Visibility + " km",
	}

	return result, nil
}

// getWindDirection 根据角度获取风向
func getWindDirection(deg int) string {
	directions := []string{"北", "东北", "东", "东南", "南", "西南", "西", "西北"}
	idx := (deg + 22) / 45 % 8
	return directions[idx]
}