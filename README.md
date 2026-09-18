# Go 电商实习项目

面向实习面试的 Go 后端项目，目标是完整覆盖后端核心知识点（HTTP、事务、缓存、消息队列、高并发、可观测性、Agent 接入）。

## 项目目标

- 覆盖后端核心面试知识点
- 包含高并发秒杀子系统（10w QPS 场景）
- 预埋 Agent 接入能力（LLM tool calling / RAG）
- 完整工程化：CI、测试、监控、链路追踪

## 快速开始

```bash
# 1. 启动依赖
docker compose -f deployments/docker-compose.yml up -d

# 2. 数据库迁移
migrate -path migrations -database "mysql://root:password@tcp(localhost:3306)/ecommerce" up

# 3. 运行服务
go run cmd/server/main.go
```

## 文档导航

- [架构规则](ARCHITECTURE.md)
- [AI 工作规则](docs/AGENT_RULES.md)
- [工程规范](CONTRIBUTING.md)

## 技术栈

| 类别 | 选型 |
|---|---|
| Web | Gin |
| ORM | GORM |
| DB | MySQL 8 |
| Cache | Redis 7 |
| MQ | Kafka |
| Auth | JWT |
| Config | Viper |
| Log | zap |
| Trace | OpenTelemetry |
| Metrics | Prometheus |
| Test | testify + gomock |
| Deploy | Docker Compose |

## 项目阶段

- [x] Phase 1 · 地基（user/product 模块、JWT、配置、日志）
- [ ] Phase 2 · 事务（订单、支付、幂等、Outbox）
- [ ] Phase 3 · 秒杀 ⭐（Redis Lua、Kafka、限流、防超卖）
- [ ] Phase 4 · 可观测性（OpenTelemetry、Grafana、单测覆盖率）
- [ ] Phase 5 · Agent 接入（tool registry、LLM 调用）
