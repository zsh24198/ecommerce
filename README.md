# Go 电商后端

一个完整的 Go 电商后端系统，覆盖订单交易、高并发秒杀、分布式稳定性保障、支付、可观测性与 AI Agent 客服接入，均为真实可运行的实现（非 Demo 骨架）。

**Go 版本要求：1.26+**（go.mod 已锁定 `go 1.26.0`，依赖链中存在要求更高 Go 版本的上游包）

## 核心亮点（简历可引用）

- **秒杀架构**：三层限流（IP/User/Global）→ Redis Lua 原子预扣 → Kafka 异步建单，实测 **1381 QPS、p99 5.84ms**
- **防超卖**：Redis Lua 合并「用户去重 SETNX + 库存 DECRBY」，消费端 event_id SETNX + DB 唯一索引双层幂等
- **分布式稳定性**：Redis 令牌桶限流、gobreaker 差异化熔断（Redis fail-closed / Kafka 降级同步建单）、Outbox 可靠投递、布隆过滤器防穿透、SingleFlight 防击穿、TTL 抖动防雪崩
- **订单与支付**：Redsync 分布式锁（升序抢锁防死锁 + 看门狗续期）、双层幂等、超时订单条件回补、HMAC-SHA256 支付验签
- **AI 智能客服 Agent**：Go + Python 双端，固定 Token 鉴权、ReAct 循环、4 个 Function Calling、RAG 检索、Redis 会话
- **可观测性**：OpenTelemetry 全链路（HTTP/Kafka/Redis/GORM）、Prometheus + Grafana、k6 阶梯加压压测

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
| AI Agent | Python + FastAPI + 百炼 LLM + ChromaDB |
| Test | testify + gomock |
| Deploy | Docker Compose |

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

## 项目阶段

- [x] Phase 1 · 地基（user/product 模块、JWT、配置、日志）
- [x] Phase 2 · 事务（订单、支付、幂等、Outbox）
- [x] Phase 3 · 秒杀 ⭐（Redis Lua、Kafka 异步建单、三层限流、差异化熔断、防超卖）
- [x] Phase 4 · 可观测性（OpenTelemetry 埋点、Prometheus 指标、Grafana 预配、单测覆盖率）
- [x] Phase 5 · Agent 接入（固定 Token 鉴权、/internal/agent/* 接口、Python 侧 LLM 工具调用 + RAG）

## 项目结构

```
cmd/server/         服务入口与依赖装配
internal/
  user/             用户与 JWT
  product/          商品与缓存三层防护（Bloom / SingleFlight / TTL 抖动）
  order/            订单主流程、幂等、超时回补、秒杀消费端
  seckill/          秒杀：Redis Lua 预扣、熔断、Kafka 投递
  payment/          支付回调与 HMAC 验签
  outbox/           Outbox 事务表与 Relay 投递
  agent/            Agent 内部接口（归属校验）
  task/             超时订单扫描
shared/
  middleware/       限流（IP/User/Global）、JWT
  cache/            Bloom / SingleFlight / JitterTTL / 泛型 CacheStore
  breaker/          gobreaker 封装与错误分类
  lock/             Redsync 分布式锁 + 看门狗
  kafka/            Producer / Consumer + trace 透传
  telemetry/        OpenTelemetry 初始化
agent/              Python Agent（FastAPI + ReAct + RAG）
```

## 已知限制与改进方向

- `outbox_events` 未存储 traceparent，Relay 使用独立根 context，HTTP → Relay → Kafka 链路存在天然断点；修复方式为新增字段并在写入/投递时 Inject/Extract
- Outbox 消费端目前仅记录日志，下游业务为占位实现，价值体现在架构预留
- 幂等占位 key 在进程崩溃时可能残留 `processing` 至 24 小时，可改为「占位短 TTL + 结果长 TTL」并轮询超时后回源 DB
- 库存锁按单 SKU 粒度，热点 SKU 仍可能成为瓶颈，可引入库存分桶
