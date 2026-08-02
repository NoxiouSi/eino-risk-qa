package llm

import (
	"context"
	"errors"
	"strings"

	"github.com/cloudwego/eino-ext/components/model/deepseek"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"

	"github.com/NoxiouSi/eino-risk-qa/internal/logging"
)

// Provider LLM 厂商标识，对应配置项 llm.provider。
type Provider string

const (
	// ProviderMock 本地/CI 专用的固定规则模拟 Provider，不依赖真实网络与 API Key。
	ProviderMock Provider = "mock"
	// ProviderOpenAI OpenAI 兼容协议（含自建/代理网关，只要遵循 Chat Completions 协议）。
	ProviderOpenAI Provider = "openai"
	// ProviderArk 火山引擎 Ark OpenAI 兼容接口，用于豆包视觉模型。
	ProviderArk Provider = "ark"
	// ProviderDeepSeek DeepSeek 官方 API（基于 eino-ext/components/model/deepseek，
	// 独立实现 ToolCallingChatModel 接口，而非借用 OpenAI 兼容通道）。
	ProviderDeepSeek Provider = "deepseek"
)

var (
	// ErrUnknownProvider 配置了未知的 provider。
	ErrUnknownProvider = errors.New("llm: unknown provider")
	// ErrMissingAPIKey 表示所选远程 Provider 未注入 API Key。
	ErrMissingAPIKey = errors.New("llm: api key is required")
)

// ProviderSupportsVision 返回当前适配器已确认的图片输入能力。
func ProviderSupportsVision(provider Provider) bool {
	return provider == ProviderOpenAI || provider == ProviderArk || provider == ProviderMock || provider == ""
}

// FactoryConfig ChatModel 工厂所需的配置（对应 configs/config.yaml 的 llm 节点）。
type FactoryConfig struct {
	Provider Provider
	OpenAI   OpenAIConfig
	Ark      ArkConfig
	DeepSeek DeepSeekConfig
}

// OpenAIConfig OpenAI 兼容协议所需的配置。
type OpenAIConfig struct {
	APIKey  string
	BaseURL string
	Model   string
}

// ArkConfig 火山引擎 Ark OpenAI 兼容接口所需配置。
type ArkConfig struct {
	APIKey  string
	BaseURL string
	Model   string
}

// DeepSeekConfig DeepSeek 官方 API 所需的配置。APIKey 建议通过环境变量注入
// （EINO_RISK_QA_LLM_DEEPSEEK_API_KEY），不写入配置文件，避免密钥落盘/入库。
type DeepSeekConfig struct {
	APIKey  string
	BaseURL string // 留空则使用 eino-ext 默认值 https://api.deepseek.com/
	Model   string // 必填，如 deepseek-chat / deepseek-reasoner
}

// NewToolCallingChatModel 按配置的 provider 分发到具体实现，统一返回
// eino 的 model.ToolCallingChatModel 接口供上层使用；切换/新增厂商仅需在此新增一个 case 分支。
func NewToolCallingChatModel(ctx context.Context, cfg FactoryConfig) (model.ToolCallingChatModel, error) {
	logging.L.Info("llm factory: constructing chat model", "provider", string(cfg.Provider))
	switch cfg.Provider {
	case ProviderMock, "":
		return NewMockChatModel(), nil
	case ProviderOpenAI:
		return newOpenAICompatibleChatModel(ctx, string(ProviderOpenAI), cfg.OpenAI.APIKey, cfg.OpenAI.BaseURL, cfg.OpenAI.Model)
	case ProviderArk:
		if strings.TrimSpace(cfg.Ark.APIKey) == "" {
			return nil, ErrMissingAPIKey
		}
		return newOpenAICompatibleChatModel(ctx, string(ProviderArk), cfg.Ark.APIKey, cfg.Ark.BaseURL, cfg.Ark.Model)
	case ProviderDeepSeek:
		cm, err := deepseek.NewChatModel(ctx, &deepseek.ChatModelConfig{
			APIKey:  cfg.DeepSeek.APIKey,
			BaseURL: cfg.DeepSeek.BaseURL,
			Model:   cfg.DeepSeek.Model,
		})
		if err != nil {
			logging.L.Error("llm factory: construct deepseek chat model failed", "model", cfg.DeepSeek.Model, "error", err.Error())
			return nil, err
		}
		return cm, nil
	default:
		logging.L.Error("llm factory: unknown provider", "provider", string(cfg.Provider))
		return nil, ErrUnknownProvider
	}
}

func newOpenAICompatibleChatModel(ctx context.Context, provider, apiKey, baseURL, modelName string) (model.ToolCallingChatModel, error) {
	cm, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		APIKey:  apiKey,
		BaseURL: baseURL,
		Model:   modelName,
	})
	if err != nil {
		logging.L.Error("llm factory: construct openai-compatible chat model failed", "provider", provider, "model", modelName, "error", err.Error())
		return nil, err
	}
	return cm, nil
}
