package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/zsh24198/ecommerce/internal/order"
	"github.com/zsh24198/ecommerce/internal/outbox"
	"github.com/zsh24198/ecommerce/internal/payment"
	"github.com/zsh24198/ecommerce/internal/product"
	"github.com/zsh24198/ecommerce/internal/user"
	"github.com/zsh24198/ecommerce/shared/config"
	"github.com/zsh24198/ecommerce/shared/jwtx"
	"github.com/zsh24198/ecommerce/shared/logger"
	"github.com/zsh24198/ecommerce/shared/middleware"
	"github.com/zsh24198/ecommerce/shared/redis"
)

func main() {
	// 1. 加载配置（logger 还没初始化，只能用标准库 log）
	cfg, err := config.Load("configs/config.yaml")
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	// 2. 初始化全局 logger（失败也只能用标准库 log，因为 logger 本身没起来）
	if err := logger.Init(cfg.Log); err != nil {
		log.Fatalf("init logger: %v", err)
	}
	defer logger.Sync()

	// 根 ctx：进程收到 SIGINT/SIGTERM 时自动 cancel，Relay 等后台任务随之优雅退出
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// 3. 连接数据库
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		cfg.MySQL.User, cfg.MySQL.Password, cfg.MySQL.Host, cfg.MySQL.Port, cfg.MySQL.DBName)
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		logger.Fatal(ctx, "connect db failed", zap.Error(err))
	}

	// 3.1 连接 Redis（用于幂等键等缓存场景）
	rdb, err := redis.New(&cfg.Redis)
	if err != nil {
		logger.Fatal(ctx, "connect redis failed", zap.Error(err))
	}

	// 4. 装配：repo → jwt → service
	repo := user.NewUserRepository(db)
	jwtMgr := jwtx.NewManager(&cfg.JWT)
	svc := user.NewUserService(repo, jwtMgr)

	// 5. Gin 引擎 + 中间件（顺序：Recovery → TraceID → CORS → Logger）
	gin.SetMode(cfg.Server.Mode)
	r := gin.New()
	r.Use(middleware.Recovery())
	r.Use(middleware.TraceID())
	r.Use(middleware.CORS())
	r.Use(middleware.Logger())

	// 6. 注册路由：构造 handler → 路由分组 → 挂载接口
	h := user.NewHandler(svc)
	users := r.Group("/api/v1/users")
	{
		users.POST("/register", h.Register)
		users.POST("/login", h.Login)
		users.POST("/refresh", h.RefreshToken)
	}

	// 6.1 商品模块装配：repo → service → handler → 路由
	pRepo := product.NewProductRepository(db)
	pSvc := product.NewProductService(pRepo)
	pHandler := product.NewHandler(pSvc)
	products := r.Group("/api/v1/products")
	{
		products.POST("", pHandler.CreateProduct)
		products.GET("", pHandler.ListProducts)
		products.GET("/:id", pHandler.GetProductDetail)
		products.PUT("/:id/status", pHandler.UpdateSPUStatus)
	}

	// 6.2 订单模块装配：repo → service（依赖 product.Service）→ handler
	// 订单路由必须登录，挂载 JWT 中间件
	oRepo := order.NewOrderRepository(db)
	oSvc := order.NewOrderService(oRepo, pSvc, rdb)
	oHandler := order.NewHandler(oSvc)
	orders := r.Group("/api/v1/orders", middleware.JWT(jwtMgr))
	{
		orders.POST("", oHandler.CreateOrder)
	}

	// 6.3 支付模块装配：signer → repo → service（依赖 order.Service）→ handler
	// 创建支付单需登录（JWT），回调接口由支付平台调用，无需 JWT，靠签名验证身份
	signer := payment.NewSigner(cfg.Payment.SignSecret)
	payRepo := payment.NewPaymentRepository(db)
	paySvc := payment.NewPaymentService(payRepo, oSvc, signer)
	payHandler := payment.NewHandler(paySvc)
	payments := r.Group("/api/v1/payments")
	{
		payments.POST("", middleware.JWT(jwtMgr), payHandler.CreatePayment)
		payments.POST("/callback", payHandler.Callback)
	}

	// 6.4 Outbox 模块装配：repo → Relay（任务 15 用 LogPublisher，任务 16 换 Kafka）→ 后台 goroutine
	outboxRepo := outbox.NewOutboxRepository(db)
	relay := outbox.NewOutboxRelay(outboxRepo, outbox.NewLogPublisher())
	go relay.Run(ctx)

	// 7. 启动 HTTP 服务（http.Server + Shutdown 支持优雅关停）
	addr := fmt.Sprintf(":%d", cfg.Server.Port)
	srv := &http.Server{Addr: addr, Handler: r}
	logger.Info(ctx, "server starting", zap.String("addr", addr))
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal(ctx, "server run failed", zap.Error(err))
		}
	}()

	// 8. 等待退出信号 → 关闭 HTTP（停止接新请求，最多等 10s 让在途请求完成）
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error(ctx, "server shutdown failed", zap.Error(err))
	}
	logger.Info(ctx, "server exited gracefully")
}
