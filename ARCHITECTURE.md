# 架构规则（强制约束）

## 总体架构：模块化单体 + 事件内核

```
cmd/server/main.go          ← 进程入口
internal/
  ├── user/                 ← 用户域
  ├── product/              ← 商品域
  ├── order/                ← 订单域
  ├── payment/              ← 支付域
  ├── seckill/              ← 秒杀域（高并发核心）
  ├── agent/                ← Agent 工具注册中心（Phase 5 启用）
  └── shared/               ← 跨域公共：errors, events, middleware, logger
pkg/                        ← 与业务无关的工具
migrations/                 ← SQL 迁移脚本
deployments/                ← docker compose / k8s
docs/                       ← 项目文档
```

## 目录规则（强制）

每个业务域目录结构固定：

```
internal/<domain>/
├── handler.go        ← HTTP 入口（Gin handler）
├── service.go        ← 业务逻辑接口
├── service_impl.go   ← 业务逻辑实现
├── repository.go     ← 数据访问接口
├── repository_impl.go← 数据访问实现（GORM）
├── model.go          ← domain model / entity
└── dto.go            ← request / response DTO
```

## 跨域调用规则（红线）

**禁止跨域直接访问对方 repository 或 model。**

- ✅ 推荐：调用对方 domain 的 `service` 接口（service facade）
- ✅ 推荐：发布 / 订阅领域事件（`shared/events`）
- ❌ 禁止：`order` 模块直接 `import "internal/product/repository"`

## 事务规则

- **单域写操作**：本地事务（`DB.Transaction`）
- **跨域写操作**：最终一致性（Outbox + Kafka）
- **Outbox 模式**：写业务表同时写 outbox 表，relay 进程异步投递到 Kafka

## 事件规则

- 事件定义在 `internal/shared/events/<domain>.go`
- 事件命名：`<Domain><Action>Event`（例：`OrderCreatedEvent`）
- 事件必须包含：event_id、occurred_at、trace_id、payload
- 消费端必须幂等（用 event_id 去重）

## 依赖注入

- 禁止在 handler / service 内直接 `gorm.Open` 或 `redis.NewClient`
- 所有外部依赖在 `cmd/server/main.go` 初始化后注入
- 使用构造函数注入，不依赖全局变量

## 配置规则

- 所有配置走 Viper，从 `configs/` 目录加载
- 支持环境：`dev` / `staging` / `prod`
- 敏感信息（DB 密码、密钥）走环境变量，**禁止硬编码**
- 配置文件示例：`configs/config.example.yaml` 进 git，真实配置进 `.gitignore`

## Agent 预埋（Phase 5 关键）

为了让 Agent 后续顺利接入，前 4 个阶段必须做到：

1. **service 层全部接口化**（本来为了单测就该做）
2. **维护 tool registry**：`internal/agent/tool_registry.go` 提供注册中心
3. **每个对外 service 方法注册 tool descriptor**（name + JSON schema + handler）
4. **领域事件可被 agent 订阅**（通过 `shared/events`）
5. **OpenAPI 自动生成**：handler 用 swag 注解，Phase 5 让 agent 自动发现 API

示例 tool descriptor 结构：

```go
type ToolDescriptor struct {
    Name        string          `json:"name"`
    Description string          `json:"description"`
    InputSchema json.RawMessage `json:"input_schema"`
    Handler     func(ctx context.Context, input json.RawMessage) (any, error)
}
```

## 扩展性设计

- 模块化单体 → 微服务：只需把每个 `internal/<domain>` 抽成独立进程
- 事件总线当前 in-memory，可平滑替换为 Kafka
- tool registry 当前本地，可平滑替换为 MCP / OpenAPI 远程调用
