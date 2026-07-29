package llm_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NoxiouSi/eino-risk-qa/internal/infra/llm"
)

func TestNewToolCallingChatModel_MockProvider(t *testing.T) {
	for _, provider := range []llm.Provider{llm.ProviderMock, ""} {
		cm, err := llm.NewToolCallingChatModel(context.Background(), llm.FactoryConfig{Provider: provider})

		require.NoError(t, err)
		assert.NotNil(t, cm)
	}
}

func TestNewToolCallingChatModel_UnknownProvider_ReturnsError(t *testing.T) {
	cm, err := llm.NewToolCallingChatModel(context.Background(), llm.FactoryConfig{Provider: "not-a-real-provider"})

	assert.Nil(t, cm)
	assert.True(t, errors.Is(err, llm.ErrUnknownProvider))
}

// DeepSeek provider 的构造过程（NewChatModel）本身不会发起真实网络调用，仅校验必填字段
// （底层 eino-ext/deepseek 组件要求 Model 非空），因此无需真实 API Key 即可完成本测试。
func TestNewToolCallingChatModel_DeepSeekProvider_ConstructsSuccessfully(t *testing.T) {
	cm, err := llm.NewToolCallingChatModel(context.Background(), llm.FactoryConfig{
		Provider: llm.ProviderDeepSeek,
		DeepSeek: llm.DeepSeekConfig{
			APIKey: "sk-test-placeholder",
			Model:  "deepseek-chat",
		},
	})

	require.NoError(t, err)
	assert.NotNil(t, cm)
}

func TestNewToolCallingChatModel_DeepSeekProvider_MissingModel_ReturnsError(t *testing.T) {
	_, err := llm.NewToolCallingChatModel(context.Background(), llm.FactoryConfig{
		Provider: llm.ProviderDeepSeek,
		DeepSeek: llm.DeepSeekConfig{APIKey: "sk-test-placeholder"},
	})

	assert.Error(t, err)
}
