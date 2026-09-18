# AI 工作规则（Trae / Cursor / Copilot 通用）

所有 AI 工具在本项目工作时，必须遵守以下规则。

## 代码风格

- 遵循 Go 官方规范：`gofmt`、`goimports`、`golangci-lint` 零警告
- 文件命名：snake_case（`order_service.go`、`user_handler.go`）
- 包命名：全小写、单数（`user`、`product`、`seckill`）
- 导出符号必须有注释，且注释以符号名开头

## 错误处理（强制）

- 统一使用 `internal/shared/apperror` 包定义业务错误
- 对外返回结构：`{"code": int, "msg": string, "data": any}`
- 错误码规则：
  - `1xxx` 客户端错误（参数、鉴权、权限）
  - `2xxx` 服务端错误
  - `3xxx` 第三方依赖错误（DB、Redis、MQ）
  - `4xxx` 业务错误（库存不足、订单状态非法）
- 禁止裸 `panic`，顶层用 `middleware.Recovery` 兜底

## 日志规范

- 使用 `internal/shared/logger`（封装 zap）
- 所有日志必须带 `trace_id`
- 关键路径必须打日志：handler 入口、service 关键步骤、DB 慢查询、外部调用
- 日志级别：debug 细节、info 关键路径、warn 业务异常、error 系统错误

## 测试要求

- service 层必须有单元测试，覆盖率 ≥ 70%
- 使用 testify + gomock mock 外部依赖
- 表驱动测试（table-driven tests）
- 测试文件命名：`<被测文件>_test.go`
- CI 必须跑 `go test ./... -race -coverprofile=coverage.out`

## 数据库规范

- 表名：snake_case 复数（`users`、`order_items`）
- 字段：snake_case，禁止缩写（`user_id` 而不是 `uid`）
- 必须有的字段：`id`、`created_at`、`updated_at`、`deleted_at`（软删除）
- 索引命名：`idx_<table>_<columns>`，唯一索引：`uk_<table>_<columns>`
- 迁移脚本必须可回滚（`migrate down`）

## API 设计规范

- RESTful 风格，资源用复数名词（`/api/v1/users`、`/api/v1/orders`）
- 版本前缀：`/api/v1/`
- 分页参数：`page`（默认 1）、`page_size`（默认 20，最大 100）
- 响应统一包装：`{"code": 0, "msg": "ok", "data": ...}`
- 错误码非 0，`msg` 描述可读信息
- handler 必须加 swag 注解（Phase 5 agent 用）

## 高并发相关（Phase 3 秒杀）

- Redis 操作必须用 Lua 脚本保证原子性
- 库存扣减顺序：Redis 预扣 → Kafka 异步落库 → 失败回滚 Redis
- 分布式锁必须设置过期时间 + 自动续期
- 所有接口必须能回答"怎么防超卖"、"怎么防雪崩"

## 安全规范

- 密码必须 bcrypt 加密，禁止 MD5 / SHA1
- JWT 密钥走环境变量
- SQL 必须参数化（GORM 默认安全，禁止 `.Raw()` 拼接用户输入）
- 用户输入必须校验（`go-playground/validator`）
- 响应禁止暴露内部错误详情（堆栈、SQL、文件路径）

## Git 提交规范

- Conventional Commits：`<type>(<scope>): <subject>`
- 常用 type：feat / fix / refactor / test / docs / chore / perf / ci
- scope 用模块名：`user`、`order`、`seckill`、`payment`
- 示例：`feat(seckill): add redis lua stock pre-deduct`

## 注释规范

- **不要加解释代码功能的注释**（代码要自解释）
- 只在以下情况加注释：
  - 复杂算法 / 业务规则
  - 为什么这么设计（trade-off）
  - 临时方案（TODO + 原因 + 链接）
- 注释语言：英文（开源惯例），文档语言：中文
