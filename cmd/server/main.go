// Command server 启动 eino-risk-qa 服务：加载配置 -> 构造 infra 层实现（LLM ChatModel适配器、
// GORM 持久化实现）-> 注入 application 层用例 -> 注入 api 层 handler -> 注册路由 -> 启动 Hertz。
// 依赖组装均为手动装配（无额外 DI 框架），符合 docs/DESIGN.md 的设计。
package main

import (
	"context"
	"flag"
	"log"

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
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "path to config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config failed: %v", err)
	}

	db, err := gorm.Open(mysql.Open(cfg.MySQL.DSN()), &gorm.Config{})
	if err != nil {
		log.Fatalf("connect mysql failed: %v", err)
	}

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
		log.Fatalf("construct chat model failed: %v", err)
	}

	judger := llm.NewJudgerAdapter(chatModel)
	sessionRepo := persistence.NewGORMSessionRepository(db)
	userBatchRepo := persistence.NewGORMUserBatchRepository(db)
	ids := idgen.NewUUIDGenerator()

	sessionSvc := application.NewSessionAppService(judger, sessionRepo)
	batchSvc := application.NewBatchAppService(sessionSvc, userBatchRepo, ids)

	batchHandler := handler.NewBatchHandler(batchSvc)
	sessionHandler := handler.NewSessionHandler(sessionSvc)

	h := server.New(server.WithHostPorts(cfg.Server.Addr))
	api.RegisterRoutes(h, cfg.Auth.APIKey, batchHandler, sessionHandler)

	log.Printf("eino-risk-qa server listening on %s (llm.provider=%s)", cfg.Server.Addr, cfg.LLM.Provider)
	h.Spin()
}
