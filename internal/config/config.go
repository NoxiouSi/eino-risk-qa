package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// Config 是服务的完整配置结构，对应 configs/config.yaml。
type Config struct {
	Server ServerConfig `mapstructure:"server"`
	MySQL  MySQLConfig  `mapstructure:"mysql"`
	LLM    LLMConfig    `mapstructure:"llm"`
	Auth   AuthConfig   `mapstructure:"auth"`
}

// ServerConfig HTTP 服务配置。
type ServerConfig struct {
	Addr string `mapstructure:"addr"`
}

// MySQLConfig 数据库连接配置。
type MySQLConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	User     string `mapstructure:"user"`
	Password string `mapstructure:"password"`
	Database string `mapstructure:"database"`
}

// DSN 拼装 GORM MySQL Driver 所需的 DSN 字符串。
func (c MySQLConfig) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		c.User, c.Password, c.Host, c.Port, c.Database)
}

// LLMConfig LLM Provider 可插拔配置，对应 docs/DESIGN.md 中的 factory.go 分发逻辑。
type LLMConfig struct {
	Provider string       `mapstructure:"provider"` // mock | openai
	OpenAI   OpenAIConfig `mapstructure:"openai"`
}

// OpenAIConfig OpenAI 兼容协议配置。
type OpenAIConfig struct {
	APIKey  string `mapstructure:"api_key"`
	BaseURL string `mapstructure:"base_url"`
	Model   string `mapstructure:"model"`
}

// AuthConfig API Key 鉴权配置。
type AuthConfig struct {
	APIKey string `mapstructure:"api_key"`
}

// Load 从指定路径加载配置文件，并支持通过环境变量覆盖（前缀 EINO_RISK_QA，
// 如 EINO_RISK_QA_MYSQL_PASSWORD 覆盖 mysql.password），未提供路径时使用内置默认值。
func Load(configPath string) (*Config, error) {
	v := viper.New()
	setDefaults(v)

	v.SetEnvPrefix("EINO_RISK_QA")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	// viper 的 AutomaticEnv 对嵌套 key 在 Unmarshal 时不会自动生效（已知行为），
	// 需要显式为每个 mapstructure key 绑定环境变量，否则环境变量覆盖不会体现在结构体上。
	bindAllEnvKeys(v)

	if configPath != "" {
		v.SetConfigFile(configPath)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("config: read config file failed: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("config: unmarshal failed: %w", err)
	}
	return &cfg, nil
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("server.addr", ":8080")
	v.SetDefault("mysql.host", "127.0.0.1")
	v.SetDefault("mysql.port", 3306)
	v.SetDefault("mysql.user", "eino_risk_qa")
	v.SetDefault("mysql.database", "eino_risk_qa")
	v.SetDefault("llm.provider", "mock")
	v.SetDefault("auth.api_key", "")
}

// bindAllEnvKeys 显式绑定全部配置项对应的环境变量，规避 AutomaticEnv 在 Unmarshal 场景下的局限。
func bindAllEnvKeys(v *viper.Viper) {
	keys := []string{
		"server.addr",
		"mysql.host", "mysql.port", "mysql.user", "mysql.password", "mysql.database",
		"llm.provider", "llm.openai.api_key", "llm.openai.base_url", "llm.openai.model",
		"auth.api_key",
	}
	for _, k := range keys {
		_ = v.BindEnv(k)
	}
}
