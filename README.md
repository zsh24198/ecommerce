# Go 电商实习项目

面向实习面试的 Go 后端项目，目标是完整覆盖后端核心知识点（HTTP、事务、缓存、消息队列、高并发、可观测性、Agent 接入）。

**Go 版本要求：1.26+**（go.mod 已锁定 `go 1.26.0`，依赖链中存在要求更高 Go 版本的上游包）

## 项目目标

- 覆盖后端核心面试知识点
- 包含高并发秒杀子系统（10w QPS 场景）
- 预埋 Agent 接入能力（LLM tool calling / RAG）
- 完整工程化：CI、测试、监控、链路追踪

## 快速开始

```bash
# 0. 前置：Go 1.26+、Docker + Docker Compose、golang-migrate
#    国内服务器请先配置 Docker 镜像加速器 + Go 代理（见「完整部署」章节）

# 1. 生成配置（configs/config.yaml 被 gitignore，必须手动创建）
cp configs/config.example.yaml configs/config.yaml
# 追加 telemetry section（example 里没写，但服务启动依赖它）
cat >> configs/config.yaml <<'EOF'

telemetry:
  enabled: true
  service_name: ecommerce-go
  exporter: stdout
  endpoint: 127.0.0.1:4318
  sample_ratio: 1.0
EOF

mkdir -p logs

# 2. 启动依赖（MySQL / Redis / Kafka / Prometheus / Grafana）
docker compose -f deployments/docker-compose.yml up -d

# 3. 数据库迁移（migrations 目录共 9 个版本，最终版本号 9）
migrate -path migrations \
  -database "mysql://ecommerce:ecommerce123456@tcp(127.0.0.1:3306)/ecommerce" up

# 4. 编译并运行
mkdir -p bin
go build -o bin/server cmd/server/main.go
./bin/server

# 5. 验证
curl http://127.0.0.1:8080/metrics   # Prometheus 指标
curl http://127.0.0.1:8080/api/v1/products
```

## 完整部署

新服务器从零部署的详细步骤（含 Docker 镜像加速、systemd 托管、可观测性验证）见
[`docs/DEPLOYMENT.md`](docs/DEPLOYMENT.md)（待补充）。

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
- [x] Phase 2 · 事务（订单、支付、幂等、Outbox）
- [x] Phase 3 · 秒杀 ⭐（Redis Lua、Kafka 异步建单、三层限流、差异化熔断、防超卖）
- [x] Phase 4 · 可观测性（OpenTelemetry 埋点、Prometheus 指标、Grafana 预配、单测覆盖率）
- [x] Phase 5 · Agent 接入（固定 Token 鉴权、/internal/agent/* 接口、Python 侧 LLM 工具调用 + RAG）
