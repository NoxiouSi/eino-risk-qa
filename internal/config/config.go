package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// Config 是服务的完整配置结构，对应 configs/config.yaml。
type Config struct {
	Server  ServerConfig  `mapstructure:"server"`
	MySQL   MySQLConfig   `mapstructure:"mysql"`
	LLM     LLMConfig     `mapstructure:"llm"`
	Storage StorageConfig `mapstructure:"storage"`
	Auth    AuthConfig    `mapstructure:"auth"`
	Log     LogConfig     `mapstructure:"log"`
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
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local&collation=utf8mb4_unicode_ci",
		c.User, c.Password, c.Host, c.Port, c.Database)
}

// LLMConfig LLM Provider 可插拔配置，对应 docs/DESIGN.md 中的 factory.go 分发逻辑。
type LLMConfig struct {
	Provider              string         `mapstructure:"provider"`
	VisionProvider        string         `mapstructure:"vision_provider"`
	RequestTimeoutSeconds int            `mapstructure:"request_timeout_seconds"`
	OpenAI                OpenAIConfig   `mapstructure:"openai"`
	Ark                   ArkConfig      `mapstructure:"ark"`
	DeepSeek              DeepSeekConfig `mapstructure:"deepseek"`
}

type StorageConfig struct {
	LocalDir            string   `mapstructure:"local_dir"`
	MaxFileBytes        int64    `mapstructure:"max_file_bytes"`
	MaxStoredImageBytes int64    `mapstructure:"max_stored_image_bytes"`
	MaxFilesPerQuestion int      `mapstructure:"max_files_per_question"`
	AllowedMIMETypes    []string `mapstructure:"allowed_mime_types"`
}

// OpenAIConfig OpenAI 兼容协议配置。
type OpenAIConfig struct {
	APIKey  string `mapstructure:"api_key"`
	BaseURL string `mapstructure:"base_url"`
	Model   string `mapstructure:"model"`
}

// ArkConfig 火山引擎 Ark OpenAI 兼容接口配置。APIKey 仅通过 ARK_API_KEY 注入。
type ArkConfig struct {
	APIKey  string `mapstructure:"api_key"`
	BaseURL string `mapstructure:"base_url"`
	Model   string `mapstructure:"model"`
}

// DeepSeekConfig DeepSeek 官方 API 配置。APIKey 约定通过环境变量
// EINO_RISK_QA_LLM_DEEPSEEK_API_KEY 注入，配置文件中留空，不落盘、不入库。
type DeepSeekConfig struct {
	APIKey  string `mapstructure:"api_key"`
	BaseURL string `mapstructure:"base_url"`
	Model   string `mapstructure:"model"`
}

// AuthConfig API Key 鉴权配置。
type AuthConfig struct {
	APIKey string `mapstructure:"api_key"`
}

// LogConfig 日志配置。
type LogConfig struct {
	Level string `mapstructure:"level"` // debug | info | warn | error，默认 info
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
	v.SetDefault("llm.vision_provider", "")
	v.SetDefault("llm.request_timeout_seconds", 300)
	v.SetDefault("llm.ark.base_url", "https://ark.cn-beijing.volces.com/api/v3")
	v.SetDefault("llm.ark.model", "doubao-seed-2-1-turbo-260628")
	v.SetDefault("llm.deepseek.model", "deepseek-chat")
	v.SetDefault("storage.local_dir", "./data/uploads")
	v.SetDefault("storage.max_file_bytes", 10*1024*1024)
	v.SetDefault("storage.max_stored_image_bytes", 1024*1024)
	v.SetDefault("storage.max_files_per_question", 5)
	v.SetDefault("storage.allowed_mime_types", []string{"image/jpeg", "image/png", "image/webp"})
	v.SetDefault("auth.api_key", "")
	v.SetDefault("log.level", "info")
}

// bindAllEnvKeys 显式绑定全部配置项对应的环境变量，规避 AutomaticEnv 在 Unmarshal 场景下的局限。
func bindAllEnvKeys(v *viper.Viper) {
	keys := []string{
		"server.addr",
		"mysql.host", "mysql.port", "mysql.user", "mysql.password", "mysql.database",
		"llm.provider", "llm.vision_provider", "llm.request_timeout_seconds", "llm.openai.api_key", "llm.openai.base_url", "llm.openai.model",
		"llm.ark.base_url", "llm.ark.model",
		"llm.deepseek.api_key", "llm.deepseek.base_url", "llm.deepseek.model",
		"storage.local_dir", "storage.max_file_bytes", "storage.max_stored_image_bytes", "storage.max_files_per_question", "storage.allowed_mime_types",
		"auth.api_key",
		"log.level",
	}
	for _, k := range keys {
		_ = v.BindEnv(k)
	}
	_ = v.BindEnv("llm.ark.api_key", "ARK_API_KEY")
}
