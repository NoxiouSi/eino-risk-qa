// Command server 启动 eino-risk-qa 服务：加载配置 -> 构造 infra 层实现（LLM ChatModel适配器、
// GORM 持久化实现）-> 注入 application 层用例 -> 注入 api 层 handler -> 注册路由 -> 启动 Hertz。
// 依赖组装均为手动装配（无额外 DI 框架），符合 docs/DESIGN.md 的设计。
package main

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"time"

	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/joho/godotenv"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	"github.com/NoxiouSi/eino-risk-qa/internal/api"
	"github.com/NoxiouSi/eino-risk-qa/internal/api/handler"
	"github.com/NoxiouSi/eino-risk-qa/internal/application"
	"github.com/NoxiouSi/eino-risk-qa/internal/config"
	"github.com/NoxiouSi/eino-risk-qa/internal/infra/idgen"
	"github.com/NoxiouSi/eino-risk-qa/internal/infra/llm"
	"github.com/NoxiouSi/eino-risk-qa/internal/infra/persistence"
	"github.com/NoxiouSi/eino-risk-qa/internal/logging"
)

func main() {
	// 加载 .env 文件（如存在），将环境变量注入当前进程，方便本地调试。
	// viper 的 AutomaticEnv() 可以读到这些注入的环境变量。
	_ = godotenv.Load(".env")

	configPath := flag.String("config", "configs/config.yaml", "path to config file")
	flag.Parse()

	resolvedPath := resolveConfigPath(*configPath)

	cfg, err := config.Load(resolvedPath)
	if err != nil {
		// 此时日志系统尚未按配置初始化（连配置都没加载成功），直接用默认 Logger 记录后退出。
		logging.L.Error("load config failed", "config_path", resolvedPath, "error", err.Error())
		os.Exit(1)
	}

	logging.Setup(cfg.Log.Level)
	logging.L.Info("config loaded", "config_path", resolvedPath, "log_level", cfg.Log.Level, "llm_provider", cfg.LLM.Provider)

	db, err := gorm.Open(mysql.Open(cfg.MySQL.DSN()), &gorm.Config{})
	if err != nil {
		logging.L.Error("connect mysql failed", "host", cfg.MySQL.Host, "port", cfg.MySQL.Port, "database", cfg.MySQL.Database, "error", err.Error())
		os.Exit(1)
	}

	// 确保底层连接使用 utf8mb4，覆盖 go-sql-driver/mysql 某些版本未正确设置 collation 的情况。
	sqlDB, _ := db.DB()
	if sqlDB != nil {
		sqlDB.SetMaxIdleConns(10)
		sqlDB.SetMaxOpenConns(100)
		sqlDB.SetConnMaxLifetime(time.Hour)
		// 首次连接后强制设置字符集，作为 DSN charset 的加固。
		if err := sqlDB.Ping(); err == nil {
			sqlDB.Exec("SET NAMES utf8mb4 COLLATE utf8mb4_unicode_ci")
		}
	}

	logging.L.Info("mysql connected", "host", cfg.MySQL.Host, "port", cfg.MySQL.Port, "database", cfg.MySQL.Database)

	chatModel, err := llm.NewToolCallingChatModel(context.Background(), modelFactoryConfig(cfg, cfg.LLM.Provider))
	if err != nil {
		logging.L.Error("construct chat model failed", "provider", cfg.LLM.Provider, "error", err.Error())
		os.Exit(1)
	}

	judger := llm.NewJudgerAdapter(chatModel)
	judger.ConfigureRequestTimeout(time.Duration(cfg.LLM.RequestTimeoutSeconds) * time.Second)
	judger.ConfigurePrimaryVisionSupport(llm.ProviderSupportsVision(llm.Provider(cfg.LLM.Provider)))
	if cfg.LLM.VisionProvider != "" {
		if !llm.ProviderSupportsVision(llm.Provider(cfg.LLM.VisionProvider)) {
			logging.L.Error("configured vision provider does not support image input", "provider", cfg.LLM.VisionProvider)
			os.Exit(1)
		}
		visionModel, visionErr := llm.NewToolCallingChatModel(context.Background(), modelFactoryConfig(cfg, cfg.LLM.VisionProvider))
		if visionErr != nil {
			logging.L.Error("construct vision model failed", "error", visionErr.Error())
			os.Exit(1)
		}
		judger.ConfigureVisionModel(visionModel)
	}

	// AttackDetector: L1 LLM 攻击意图判别器，复用主 chatModel（初期不单独配置模型）
	atkCfg := cfg.LLM.AttackDetector
	attackDetector := llm.NewAttackDetector(chatModel, llm.AttackDetectorConfig{
		Enabled:             atkCfg.Enabled,
		ConfidenceThreshold: atkCfg.ConfidenceThreshold,
		TimeoutSeconds:      atkCfg.TimeoutSeconds,
	})
	if atkCfg.Enabled {
		logging.L.Info("attack detector enabled",
			"confidence_threshold", atkCfg.ConfidenceThreshold,
			"timeout_seconds", atkCfg.TimeoutSeconds,
		)
	} else {
		logging.L.Info("attack detector disabled")
	}
	judger.ConfigureAttackDetector(attackDetector)

	sessionRepo := persistence.NewGORMSessionRepository(db)
	userBatchRepo := persistence.NewGORMUserBatchRepository(db)
	questionCatalog := persistence.NewGORMMainQuestionRepository(db)
	attachmentRepo := persistence.NewGORMAttachmentRepository(db)
	ids := idgen.NewUUIDGenerator()

	sessionSvc := application.NewSessionAppService(judger, sessionRepo)
	sessionSvc.ConfigureQuestionSupport(questionCatalog, attachmentRepo, cfg.Storage.LocalDir, cfg.Storage.MaxFilesPerQuestion)
	batchSvc := application.NewBatchAppService(sessionSvc, userBatchRepo, ids)
	userSvc := application.NewUserAppService(userBatchRepo, questionCatalog)

	batchHandler := handler.NewBatchHandler(batchSvc)
	sessionHandler := handler.NewSessionHandler(sessionSvc)
	userHandler := handler.NewUserHandler(userSvc)
	attachmentHandler := handler.NewAttachmentHandler(attachmentRepo, cfg.Storage, questionCatalog)

	h := server.New(server.WithHostPorts(cfg.Server.Addr))
	api.RegisterRoutes(h, cfg.Auth.APIKey, batchHandler, sessionHandler, userHandler, attachmentHandler)

	logging.L.Info("eino-risk-qa server listening", "addr", cfg.Server.Addr, "llm_provider", cfg.LLM.Provider)
	h.Spin()
}

// resolveConfigPath 解析配置文件路径，支持相对路径和绝对路径。
// 当给定的相对路径不存在时，依次尝试：基于可执行文件目录、从当前目录向上搜索 go.mod 定位项目根目录。
// 兼容 CodeBuddy 调试、go run、直接运行编译产物等不同工作目录下的运行场景。
func resolveConfigPath(rawPath string) string {
	// 绝对路径或文件已存在 -> 直接使用
	if filepath.IsAbs(rawPath) {
		return rawPath
	}
	if _, err := os.Stat(rawPath); err == nil {
		return rawPath
	}
	// 尝试基于可执行文件目录解析（直接运行编译产物时有效）
	if execPath, err := os.Executable(); err == nil {
		resolved := filepath.Join(filepath.Dir(execPath), rawPath)
		if _, err := os.Stat(resolved); err == nil {
			return resolved
		}
	}
	// 从当前工作目录向上搜索 go.mod，定位项目根目录（go run / dlv debug 场景有效）
	if cwd, err := os.Getwd(); err == nil {
		for dir := cwd; dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				resolved := filepath.Join(dir, rawPath)
				if _, err := os.Stat(resolved); err == nil {
					return resolved
				}
				break
			}
		}
	}
	// 都找不到则返回原始路径，让下游 Load 产生可读的错误信息
	return rawPath
}

func modelFactoryConfig(cfg *config.Config, provider string) llm.FactoryConfig {
	return llm.FactoryConfig{
		Provider: llm.Provider(provider),
		OpenAI: llm.OpenAIConfig{
			APIKey:  cfg.LLM.OpenAI.APIKey,
			BaseURL: cfg.LLM.OpenAI.BaseURL,
			Model:   cfg.LLM.OpenAI.Model,
		},
		Ark: llm.ArkConfig{
			APIKey:  cfg.LLM.Ark.APIKey,
			BaseURL: cfg.LLM.Ark.BaseURL,
			Model:   cfg.LLM.Ark.Model,
		},
		DeepSeek: llm.DeepSeekConfig{
			APIKey:  cfg.LLM.DeepSeek.APIKey,
			BaseURL: cfg.LLM.DeepSeek.BaseURL,
			Model:   cfg.LLM.DeepSeek.Model,
		},
	}
}
