# 工程规范（Git / CI / PR）

## 分支策略（轻量 GitFlow）

```
main          ← 稳定版本，永远可运行
  ↑
develop       ← 日常集成分支
  ↑
feat/xxx      ← 功能分支（从 develop 切出）
fix/xxx       ← bug 修复分支
```

- `main` 禁直推，必须走 PR
- 功能开发从 `develop` 切 `feat/<scope>-<desc>`
- 修复 bug 切 `fix/<scope>-<desc>`
- 紧急 hotfix 直接 `hotfix/<desc>` → `main`

## Commit Message 规范

格式：`<type>(<scope>): <subject>`

```
feat(order): add order creation with idempotent key
fix(seckill): prevent oversold under high concurrency
refactor(user): split jwt service into token and refresh
test(product): add service layer unit tests, coverage 80%
docs(readme): add architecture overview
chore(deps): bump gorm to v1.25.5
ci(actions): add golangci-lint workflow
perf(seckill): use pipeline to batch redis ops
```

**type 速查**：

| type | 用途 |
|---|---|
| feat | 新功能 |
| fix | 修 bug |
| refactor | 重构（不改行为） |
| test | 加测试 |
| docs | 文档 |
| chore | 杂项（依赖、配置） |
| perf | 性能优化 |
| ci | CI 配置 |
| build | 构建系统 |

**scope 用模块名**：`user`、`product`、`order`、`payment`、`seckill`、`shared`

## Pull Request 规范

- 标题格式同 commit message
- 描述必须包含：
  - 改动内容（What）
  - 为什么改（Why）
  - 怎么验证（How）
- CI 必须全绿才能合
- 自己写的 PR 也建议留 review 痕迹

## GitHub Actions CI

`.github/workflows/ci.yml` 必须包含：

```yaml
name: CI
on:
  pull_request:
    branches: [main, develop]
  push:
    branches: [main, develop]

jobs:
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - name: golangci-lint
        uses: golangci/golangci-lint-action@v6
        with:
          version: latest

  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - name: Run tests
        run: go test -race -coverprofile=coverage.out ./...
      - name: Upload coverage
        uses: codecov/codecov-action@v4

  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - name: Build
        run: go build -o bin/server ./cmd/server
```

## 版本打 Tag

按阶段打 tag，方便演示演进过程：

```
v0.1.0  Phase 1 地基（user/product/JWT/配置/日志）
v0.2.0  Phase 2 事务（订单/支付/幂等/Outbox）
v0.3.0  Phase 3 秒杀 ⭐
v0.4.0  Phase 4 可观测性（OTel/Grafana/测试覆盖）
v0.5.0  Phase 5 Agent 接入
```

打 tag 命令：

```bash
git tag -a v0.1.0 -m "Phase 1: foundation - user/product/JWT"
git push origin v0.1.0
```

## 本地开发环境

### 必备工具

- Go 1.22+
- Docker + Docker Compose
- golangci-lint
- migrate（数据库迁移）

### 安装 lint

```bash
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

### 提交前自检

```bash
# 1. 格式化
gofmt -w .
goimports -w .

# 2. lint
golangci-lint run

# 3. 测试
go test -race ./...

# 4. 构建
go build ./...
```

## .gitignore 关键项

```gitignore
# Binaries
*.exe
/bin/
/cmd/server/server

# Go workspace
go.work
go.work.sum

# IDE
.idea/
.vscode/

# Env & secrets
.env
.env.local
*.pem
*.key

# Logs & runtime
logs/
*.log
tmp/

# Test coverage
coverage.out
coverage.html

# Local DB data
mysql_data/
redis_data/
kafka_data/
```

## 推荐 IDE 配置

### VSCode settings.json

```json
{
  "go.lintTool": "golangci-lint",
  "go.lintOnSave": "package",
  "go.formatTool": "goimports",
  "go.testOnSave": true,
  "go.coverOnSave": true
}
```
