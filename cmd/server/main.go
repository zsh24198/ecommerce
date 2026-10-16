package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/zsh24198/ecommerce/internal/agent"
	"github.com/zsh24198/ecommerce/internal/order"
	"github.com/zsh24198/ecommerce/internal/outbox"
	"github.com/zsh24198/ecommerce/internal/payment"
	"github.com/zsh24198/ecommerce/internal/product"
	"github.com/zsh24198/ecommerce/internal/seckill"
	"github.com/zsh24198/ecommerce/internal/user"
	"github.com/zsh24198/ecommerce/shared/breaker"
	"github.com/zsh24198/ecommerce/shared/cache"
	"github.com/zsh24198/ecommerce/shared/config"
	"github.com/zsh24198/ecommerce/shared/jwtx"
	"github.com/zsh24198/ecommerce/shared/kafka"
	"github.com/zsh24198/ecommerce/shared/lock"
	"github.com/zsh24198/ecommerce/shared/logger"
	"github.com/zsh24198/ecommerce/shared/middleware"
	"github.com/zsh24198/ecommerce/shared/redis"
	"github.com/zsh24198/ecommerce/shared/telemetry"
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

	// 2.1 初始化 OpenTelemetry：必须早于 db（GORM/Redis 埋点依赖全局 TracerProvider）。
	// 用 Background 而非根 ctx：初始化不随请求生命周期取消。
	shutdownTracer, err := telemetry.Init(context.Background(), cfg.Telemetry)
	if err != nil {
		logger.Fatal(ctx, "init telemetry failed", zap.Error(err))
	}
	defer func() {
		// 退出时根 ctx 已 cancel，必须用新 context 给 Trace flush 留时间
		flushCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownTracer(flushCtx); err != nil {
			logger.Error(ctx, "tracer shutdown failed", zap.Error(err))
		}
	}()

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
	r.Use(otelgin.Middleware(cfg.Telemetry.ServiceName))
	r.Use(middleware.TraceID())
	r.Use(middleware.Prometheus())
	r.Use(middleware.CORS())
	r.Use(middleware.Logger())

	// 6. 注册路由：构造 handler → 路由分组 → 挂载接口
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	h := user.NewHandler(svc)
	users := r.Group("/api/v1/users")
	{
		users.POST("/register", h.Register)
		users.POST("/login", h.Login)
		users.POST("/refresh", h.RefreshToken)
	}

	// 6.1 商品模块装配：repo → cache/bloom/sf → service → handler → 路由
	pRepo := product.NewProductRepository(db)

	// 6.1.1 布隆过滤器启动预热（A 方案）：全量加载 SPU ID。
	// 容量预估 10w（预期 5w 商品 × 2 倍余量），误判率 0.1%（0.001）。
	// pRepo 在上一步已构造，这里跨调用复用。
	pBloom := cache.NewBloomFilter(100000, 0.001)
	if ids, err := pRepo.ListAllSPUIDs(ctx); err != nil {
		logger.Warn(ctx, "bloom warmup failed, will start with empty bloom",
			zap.Error(err))
	} else {
		if err := pBloom.LoadBulk(ctx, ids); err != nil {
			logger.Warn(ctx, "bloom load bulk failed", zap.Error(err))
		} else {
			logger.Info(ctx, "bloom warmup done",
				zap.Int("loaded", len(ids)))
		}
	}

	// 6.1.2 Redis CacheStore 构造。decode 由业务方注入：cache 包不认识 *ProductDetail。
	pCache := cache.NewRedisCache[*product.ProductDetail](rdb.Client, func(data []byte) (*product.ProductDetail, error) {
		var d product.ProductDetail
		if err := json.Unmarshal(data, &d); err != nil {
			return nil, err
		}
		return &d, nil
	})

	// 6.1.3 SingleFlight 全局共享实例。
	sf := cache.NewSingleFlight()

	pSvc := product.NewProductService(pRepo, pCache, pBloom, sf)
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
	// lock.Manager 与订单服务共用同一个 Redis 连接池；ttl=0 用默认 30s
	locker := lock.NewManager(rdb.Client, 0)
	oRepo := order.NewOrderRepository(db)
	oSvc := order.NewOrderService(oRepo, pSvc, rdb, locker)
	oHandler := order.NewHandler(oSvc)
	orders := r.Group("/api/v1/orders", middleware.JWT(jwtMgr))
	{
		orders.POST("", oHandler.CreateOrder)
		orders.GET("/seckill/status", oHandler.GetSeckillOrderStatus)
		orders.POST("/:order_no/receiver", oHandler.FillReceiverInfo)
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

	// 6.4 Outbox 模块装配：repo → Relay（KafkaPublisher 投递到 Kafka）→ 后台 goroutine
	kafkaProducer, err := kafka.NewProducer(&cfg.Kafka)
	if err != nil {
		logger.Fatal(ctx, "connect kafka failed", zap.Error(err))
	}
	defer kafkaProducer.Close()

	outboxRepo := outbox.NewOutboxRepository(db)
	relay := outbox.NewOutboxRelay(outboxRepo, outbox.NewKafkaPublisher(kafkaProducer, "ecommerce."))
	go relay.Run(ctx)

	// 6.5 订单事件消费者：Kafka 消费组 + event_id 去重（后台 goroutine）
	eventHandler := order.NewEventHandler(rdb)
	consumerGroup, err := kafka.NewConsumerGroup(&cfg.Kafka, []string{"ecommerce.order"}, eventHandler.Handle)
	if err != nil {
		logger.Fatal(ctx, "start kafka consumer failed", zap.Error(err))
	}
	go consumerGroup.Run(ctx)
	defer consumerGroup.Close()

	// 6.6 熔断降级组件：Redis/Kafka 各一套
	// Redis 熔断：5 次连续依赖故障 → 打开，30s 后探测（fail-closed，保护 DB）
	redisBreaker := breaker.New(breaker.DefaultConfig("seckill_redis"))
	// Kafka 熔断：5 次连续发送失败 → 打开，30s 后探测（打开时降级同步写订单）
	kafkaBreaker := breaker.New(breaker.DefaultConfig("seckill_kafka"))

	// 6.7 秒杀模块装配：repo → service（依赖 product.Service + orderService + breaker）→ handler
	sRepo := seckill.NewSeckillRepository(db)
	sCache := seckill.NewActivityCache(sRepo, sf)
	sStock := seckill.NewStockStore(rdb)
	sSvc := seckill.NewSeckillService(sRepo, pSvc, sCache, sStock, kafkaProducer, oSvc, redisBreaker, kafkaBreaker)
	sHandler := seckill.NewHandler(sSvc)
	seckillGrp := r.Group("/api/v1/seckill")
	{
		seckillGrp.POST("/activities", sHandler.CreateActivity)
		seckillGrp.GET("/activities", sHandler.ListActivities)
		seckillGrp.GET("/ongoing", sHandler.ListOngoingActivities)
		seckillGrp.GET("/activities/:id", sHandler.GetActivity)
		seckillGrp.PUT("/activities/:id/enabled", sHandler.UpdateActivityEnabled)
		// 秒杀抢购入口：三层限流叠加（顺序不能错）
		// IP 限流(无需 user_id,最早短路) → JWT(消耗 CPU,放在 IP 之后) → 用户限流(需 user_id,必须在 JWT 之后) → 全局兜底(最后)
		seckillAuth := seckillGrp.Group("",
			middleware.PerKeyRateLimit(rdb, 10, 5, middleware.IPKeyFunc()),
			middleware.JWT(jwtMgr),
			middleware.PerKeyRateLimit(rdb, 2, 1, middleware.UserKeyFunc()),
			middleware.GlobalRateLimit(5000, 8000),
		)
		{
			seckillAuth.POST("/activities/:id/buy", sHandler.Buy)
		}
	}

	// 6.7 秒杀下单消费者：订阅 seckill-order 主题，异步建单。
	// SeckillStockRestorer 由 seckill.StockStore 隐式实现（鸭子类型），
	// order 包只依赖接口，不 import seckill 具体类型。
	seckillConsumer := order.NewSeckillOrderConsumer(oSvc, rdb, sStock)
	seckillConsumerGroup, err := kafka.NewConsumerGroup(&cfg.Kafka, []string{seckill.KafkaTopicSeckillOrder}, seckillConsumer.Handle)
	if err != nil {
		logger.Fatal(ctx, "start seckill order consumer failed", zap.Error(err))
	}
	go seckillConsumerGroup.Run(ctx)
	defer seckillConsumerGroup.Close()

	// 6.8 Agent 专用 API：面向 Python Agent，复用各业务域 Service 接口，参数极简。
	// 鉴权用固定 Agent Token（X-Agent-Token 头），与用户 JWT 分流。
	agentHandler := agent.NewHandler(svc, pSvc, oSvc)
	agentGrp := r.Group("/internal/agent", agent.AgentAuth(cfg.Agent.Token))
	{
		agentGrp.POST("/orders/query", agentHandler.QueryOrder)
		agentGrp.POST("/products/query", agentHandler.QueryProduct)
		agentGrp.POST("/users/query", agentHandler.QueryUser)
	}

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
