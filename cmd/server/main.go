// Command server 启动 eino-risk-qa 服务：加载配置 -> 构造 infra 层实现（LLM ChatModel适配器、
// GORM 持久化实现）-> 注入 application 层用例 -> 注入 api 层 handler -> 注册路由 -> 启动 Hertz。
// 依赖组装均为手动装配（无额外 DI 框架），符合 docs/DESIGN.md 的设计。
package main

import (
	"context"
	"flag"
	"os"

	"github.com/cloudwego/hertz/pkg/app/server"
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
	configPath := flag.String("config", "configs/config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		// 此时日志系统尚未按配置初始化（连配置都没加载成功），直接用默认 Logger 记录后退出。
		logging.L.Error("load config failed", "error", err.Error())
		os.Exit(1)
	}

	logging.Setup(cfg.Log.Level)
	logging.L.Info("config loaded", "config_path", *configPath, "log_level", cfg.Log.Level, "llm_provider", cfg.LLM.Provider)

	db, err := gorm.Open(mysql.Open(cfg.MySQL.DSN()), &gorm.Config{})
	if err != nil {
		logging.L.Error("connect mysql failed", "host", cfg.MySQL.Host, "port", cfg.MySQL.Port, "database", cfg.MySQL.Database, "error", err.Error())
		os.Exit(1)
	}
	logging.L.Info("mysql connected", "host", cfg.MySQL.Host, "port", cfg.MySQL.Port, "database", cfg.MySQL.Database)

	chatModel, err := llm.NewToolCallingChatModel(context.Background(), llm.FactoryConfig{
		Provider: llm.Provider(cfg.LLM.Provider),
		OpenAI: llm.OpenAIConfig{
			APIKey:  cfg.LLM.OpenAI.APIKey,
			BaseURL: cfg.LLM.OpenAI.BaseURL,
			Model:   cfg.LLM.OpenAI.Model,
		},
		DeepSeek: llm.DeepSeekConfig{
			APIKey:  cfg.LLM.DeepSeek.APIKey,
			BaseURL: cfg.LLM.DeepSeek.BaseURL,
			Model:   cfg.LLM.DeepSeek.Model,
		},
	})
	if err != nil {
		logging.L.Error("construct chat model failed", "provider", cfg.LLM.Provider, "error", err.Error())
		os.Exit(1)
	}

	judger := llm.NewJudgerAdapter(chatModel)
	sessionRepo := persistence.NewGORMSessionRepository(db)
	userBatchRepo := persistence.NewGORMUserBatchRepository(db)
	mainQuestionRepo := persistence.NewGORMMainQuestionRepository(db)
	ids := idgen.NewUUIDGenerator()

	sessionSvc := application.NewSessionAppService(judger, sessionRepo)
	batchSvc := application.NewBatchAppService(sessionSvc, userBatchRepo, ids)
	userSvc := application.NewUserAppService(userBatchRepo, mainQuestionRepo)

	batchHandler := handler.NewBatchHandler(batchSvc)
	sessionHandler := handler.NewSessionHandler(sessionSvc)
	userHandler := handler.NewUserHandler(userSvc)

	h := server.New(server.WithHostPorts(cfg.Server.Addr))
	api.RegisterRoutes(h, cfg.Auth.APIKey, batchHandler, sessionHandler, userHandler)

	logging.L.Info("eino-risk-qa server listening", "addr", cfg.Server.Addr, "llm_provider", cfg.LLM.Provider)
	h.Spin()
}
