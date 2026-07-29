package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NoxiouSi/eino-risk-qa/internal/config"
)

func TestLoad_NoConfigFile_UsesDefaults(t *testing.T) {
	cfg, err := config.Load("")

	require.NoError(t, err)
	assert.Equal(t, ":8080", cfg.Server.Addr)
	assert.Equal(t, "mock", cfg.LLM.Provider)
	assert.Equal(t, "eino_risk_qa", cfg.MySQL.Database)
}

func TestLoad_FromYAMLFile_OverridesDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yaml := `
server:
  addr: ":9090"
mysql:
  host: "db.internal"
  port: 3307
  user: "app_user"
  password: "secret"
  database: "eino_risk_qa_prod"
llm:
  provider: "openai"
  openai:
    api_key: "sk-test"
    base_url: "https://api.example.com/v1"
    model: "gpt-4o-mini"
  deepseek:
    base_url: "https://api.deepseek.com/beta"
    model: "deepseek-reasoner"
auth:
  api_key: "my-secret-key"
`
	require.NoError(t, os.WriteFile(path, []byte(yaml), 0o600))

	cfg, err := config.Load(path)

	require.NoError(t, err)
	assert.Equal(t, ":9090", cfg.Server.Addr)
	assert.Equal(t, "db.internal", cfg.MySQL.Host)
	assert.Equal(t, 3307, cfg.MySQL.Port)
	assert.Equal(t, "openai", cfg.LLM.Provider)
	assert.Equal(t, "sk-test", cfg.LLM.OpenAI.APIKey)
	assert.Equal(t, "deepseek-reasoner", cfg.LLM.DeepSeek.Model)
	assert.Equal(t, "https://api.deepseek.com/beta", cfg.LLM.DeepSeek.BaseURL)
	assert.Equal(t, "my-secret-key", cfg.Auth.APIKey)
}

func TestLoad_NoConfigFile_DeepSeekModelDefaultsToDeepSeekChat(t *testing.T) {
	cfg, err := config.Load("")

	require.NoError(t, err)
	assert.Equal(t, "deepseek-chat", cfg.LLM.DeepSeek.Model)
	assert.Empty(t, cfg.LLM.DeepSeek.APIKey)
}

func TestLoad_DeepSeekAPIKey_InjectedViaEnvironmentVariable(t *testing.T) {
	t.Setenv("EINO_RISK_QA_LLM_DEEPSEEK_API_KEY", "sk-from-env")

	cfg, err := config.Load("")

	require.NoError(t, err)
	assert.Equal(t, "sk-from-env", cfg.LLM.DeepSeek.APIKey)
}

func TestLoad_EnvironmentVariableOverridesFileAndDefault(t *testing.T) {
	t.Setenv("EINO_RISK_QA_MYSQL_PASSWORD", "from-env")
	t.Setenv("EINO_RISK_QA_AUTH_API_KEY", "env-key")

	cfg, err := config.Load("")

	require.NoError(t, err)
	assert.Equal(t, "from-env", cfg.MySQL.Password)
	assert.Equal(t, "env-key", cfg.Auth.APIKey)
}

func TestMySQLConfig_DSN_FormatsCorrectly(t *testing.T) {
	m := config.MySQLConfig{Host: "127.0.0.1", Port: 3306, User: "u", Password: "p", Database: "d"}

	assert.Equal(t, "u:p@tcp(127.0.0.1:3306)/d?charset=utf8mb4&parseTime=True&loc=Local", m.DSN())
}
