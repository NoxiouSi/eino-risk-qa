package llm

import (
	"context"
	"errors"

	"github.com/cloudwego/eino-ext/components/model/deepseek"
	"github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
)

// Provider LLM 厂商标识，对应配置项 llm.provider。
type Provider string

const (
	// ProviderMock 本地/CI 专用的固定规则模拟 Provider，不依赖真实网络与 API Key。
	ProviderMock Provider = "mock"
	// ProviderOpenAI OpenAI 兼容协议（含自建/代理网关，只要遵循 Chat Completions 协议）。
	ProviderOpenAI Provider = "openai"
	// ProviderDeepSeek DeepSeek 官方 API（基于 eino-ext/components/model/deepseek，
	// 独立实现 ToolCallingChatModel 接口，而非借用 OpenAI 兼容通道）。
	ProviderDeepSeek Provider = "deepseek"
)

// ErrUnknownProvider 配置了未知的 provider。
var ErrUnknownProvider = errors.New("llm: unknown provider")

// FactoryConfig ChatModel 工厂所需的配置（对应 configs/config.yaml 的 llm 节点）。
type FactoryConfig struct {
	Provider Provider
	OpenAI   OpenAIConfig
	DeepSeek DeepSeekConfig
}

// OpenAIConfig OpenAI 兼容协议所需的配置。
type OpenAIConfig struct {
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
	switch cfg.Provider {
	case ProviderMock, "":
		return NewMockChatModel(), nil
	case ProviderOpenAI:
		return openai.NewChatModel(ctx, &openai.ChatModelConfig{
			APIKey:  cfg.OpenAI.APIKey,
			BaseURL: cfg.OpenAI.BaseURL,
			Model:   cfg.OpenAI.Model,
		})
	case ProviderDeepSeek:
		return deepseek.NewChatModel(ctx, &deepseek.ChatModelConfig{
			APIKey:  cfg.DeepSeek.APIKey,
			BaseURL: cfg.DeepSeek.BaseURL,
			Model:   cfg.DeepSeek.Model,
		})
	default:
		return nil, ErrUnknownProvider
	}
}
