# Job Atlas Go 质量门禁研究

> 对应议题：[研究 golangci-lint 复杂度规则与统一质量门禁](https://github.com/Russell-Utopia/job-atlas/issues/29)
>
> 测量基线：`1d8c98f68070e2c6af47d25321a6912092b726a6`
>
> 测量环境：macOS arm64，Go `1.23.3`，golangci-lint `v2.13.2`，govulncheck `v1.1.4`
>
> 本文只给出事实、推断和候选建议；最终工具与阈值仍由架构决策确定。

## 结论摘要

### 已确认的事实

- 当前仓库只有一个 Go package，3 个 Go 文件、832 行（含测试、空行、注释和内嵌 SQL），3 个测试；测试覆盖真实 SQLite、后台 goroutine 和重启恢复，语义上已经包含集成测试。
- `gofmt`、普通测试、Race Detector、`go vet`、`go mod verify`、`go mod tidy -diff` 均通过；语句覆盖率为 76.5%。
- golangci-lint v2 的默认集合不是“复杂度检查”。默认启用的是 `errcheck`、`govet`、`ineffassign`、`staticcheck`、`unused`；当前代码因 4 处未检查的 `Close` / `Rollback` 返回值失败。[官方 Quick Start](https://golangci-lint.run/docs/welcome/quick-start/)
- 当前生产代码的圈复杂度最高为 9；默认 `cyclop=10` 不会拦截生产代码。越界的是 `TestUnfinishedDiscoveryContinuesAfterServiceRestart`：cyclop 18、gocyclo 14、funlen 68 行 / 34 条语句、maintidx 35。
- `govulncheck ./...` 在当前 Go `1.23.3` 标准库中找到 3 条有调用路径的漏洞；这说明“依赖安全”不只检查 `go.mod`，还必须包含实际构建用的 Go 工具链。

### 基于事实的推断

- 当前复杂度风险主要是：后续 AI 继续向单一 package 和少数编排/SQLite 函数追加分支；现在不是大规模重构现有函数的证据。
- `cyclop` 与 `gocyclo` 都在衡量圈复杂度，长期同时开启会在同一位置产生重复信号。`cyclop` 对 `default` case / select default 的计数更严格，并额外提供 package average；两者应二选一。
- 当前最长测试是一个完整恢复场景。若让它与生产函数共用同一阈值，要么今天就拆散一个仍然可读的场景，要么把生产阈值放宽到 18 以上。因此更精确的做法是只豁免顶层 `func Test...` 的结构指标，同时继续检查测试 helper，而不是排除整个 `_test.go`。
- `maintidx` 把圈复杂度、Halstead Volume 和源码行数合成一个整数；作者明确称其为 experimental。它适合作为第二信号，不适合在第一天单独决定函数必须拆分。

### 候选方向（不是最终选型）

1. 使用一个仓库级 `make check` 作为合并门禁；CI 只调用这个入口，不在 workflow 中复制检查命令。可另设 `make check-fast` 供编辑循环使用，但它不能替代完整门禁。
2. 固定 Go、golangci-lint、govulncheck 的版本；golangci-lint 官方也明确建议 CI 使用固定版本，避免上游新增/升级 linter 让全部构建同时失败。[官方 CI 安装说明](https://golangci-lint.run/docs/welcome/install/ci/)
3. 明确列出启用的 linter，不使用 `linters.default: all`；若显式运行 `go vet ./...`，则 golangci-lint 中不要再启用 `govet`。
4. 复杂度硬门禁的起始候选为：`cyclop.max-complexity: 12`、`funlen.lines: 60`、`funlen.statements: 30`；只对顶层 `func Test...` 豁免这两项。`maintidx.under: 40` 先观察一轮，再决定是否成为硬门禁。
5. 在启用完整门禁前，先处理 4 个 `errcheck` 结果并升级到无可达标准库漏洞的受支持 Go patch；不要用宽泛 exclude 把基线“刷绿”。

## 当前仓库基线

### 仓库形态

| 项目 | 当前值 |
| --- | --- |
| Go module | `github.com/Russell-Utopia/job-atlas` |
| `go` directive | `1.23.0` |
| 实际 Go | `go1.23.3 darwin/arm64` |
| package | 1 个：`jobatlas` |
| Go 文件 | `discovery.go` 301 行；`sqlite_store.go` 281 行；`discovery_test.go` 250 行 |
| 测试 | 3 个；真实 SQLite + 后台 worker + 重启恢复 |
| 直接模块依赖 | `modernc.org/sqlite v1.38.2` |

### 可复现命令与结果

所有命令均在上述 commit 的隔离 worktree 中执行；`-count=1` 用来避免成功测试缓存。Go 官方说明 package-list 模式会缓存成功结果，而 `-count=1` 可显式禁用缓存。[`go test` 官方说明](https://pkg.go.dev/cmd/go#hdr-Test_packages)

| 命令 | 结果 |
| --- | --- |
| `gofmt -l discovery.go discovery_test.go sqlite_store.go` | 无输出，通过 |
| `go test -count=1 ./...` | 通过；测试耗时 0.031s，墙钟约 1.05s |
| `go test -race -count=1 ./...` | 通过；测试耗时 1.104s，墙钟约 2.03s |
| `go vet ./...` | 无诊断，通过 |
| `go mod verify` | `all modules verified` |
| `go mod tidy -diff` | 无 diff，通过 |
| `go test -count=1 -cover ./...` | 通过；statement coverage 76.5% |
| `golangci-lint v2.13.2 run --no-config ./...` | 失败；4 个 `errcheck` |
| `govulncheck v1.1.4 ./...` | 失败；3 条有调用路径的标准库漏洞 |

golangci-lint 的 4 个现存结果：

- `sqlite_store.go:97`：`rows.Close` 返回值未检查；
- `sqlite_store.go:129`：`tx.Rollback` 返回值未检查；
- `sqlite_store.go:192`：`tx.Rollback` 返回值未检查；
- `sqlite_store.go:237`：`tx.Rollback` 返回值未检查。

这 4 条只是事实，不意味着统一采用同一种修复。事务成功提交后的延迟回滚与真正失败的清理错误语义不同，实施票应分别决定“处理”或“明确忽略”，不能由本研究改业务代码。

### 当前漏洞扫描结果

govulncheck 使用静态调用分析缩小到实际可能影响程序的符号；它与“模块版本出现在漏洞库中”不是同一口径。[govulncheck 官方说明](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck)

| 漏洞 | 当前调用路径 | 官方修复范围 |
| --- | --- | --- |
| [GO-2026-4341](https://pkg.go.dev/vuln/GO-2026-4341) `net/url` 参数解析内存耗尽 | `sql.Open` 最终调用 `url.ParseQuery` | govulncheck 对本环境提示修复于 Go 1.24.12；官方记录为 1.24.12 前、1.25.0–1.25.5 受影响 |
| [GO-2025-3849](https://pkg.go.dev/vuln/GO-2025-3849) `database/sql.Rows.Scan` 错误结果 | 当前 `Row.Scan`、`Rows.Scan` | Go 1.23.12 / 1.24.6 起修复 |
| [GO-2025-3750](https://pkg.go.dev/vuln/GO-2025-3750) `O_CREATE\|O_EXCL` 跨平台不一致 | 经 `os` / `syscall` 的可达路径；仅 Windows 平台受影响 | Go 1.23.10 / 1.24.4 起修复 |

扫描还找到“10 条位于已导入 package、43 条位于所需 module、但当前代码没有调用”的漏洞。完整门禁应阻断有调用路径的结果；是否同时把“仅存在但不可达”作为提醒，需要单独制定安全策略。

## 各检查层分别解决什么问题

| 层 | 解决的问题 | 不解决的问题 | 与其他层重叠 |
| --- | --- | --- | --- |
| `gofmt` / `golangci-lint fmt --diff` | 唯一、机械化的源码排版；`--diff` 可在 CI 非零退出 | 不判断正确性、命名、复杂度 | 与普通 lint 不重叠；golangci-lint v2 已把 formatter 与 linter 分区。[CLI](https://golangci-lint.run/docs/configuration/cli/) |
| `go test -count=1 ./...` | 编译 package 与测试，并运行全部匹配测试 | 未执行路径、数据竞争、外部平台真实行为 | 会运行一组高置信度 vet 子集，但不是完整 `go vet`。[Go 命令说明](https://pkg.go.dev/cmd/go#hdr-Test_packages) |
| `go test -race -count=1 ./...` | 发现测试实际执行路径中的并发读写冲突 | 未覆盖路径中的 race | 重复执行测试，但增加动态插桩；官方给出的典型成本为 5–10 倍内存、2–20 倍时间。[Race Detector](https://go.dev/doc/articles/race_detector) |
| `go vet ./...` | 用官方 analyzers 查找可疑结构，例如 Printf 参数、copylocks、lostcancel 等 | 不保证所有报告都是真问题，也不覆盖所有 bug | `go test` 只跑其高置信子集；golangci 的 `govet` “roughly the same”，二者无需重复。[cmd/vet](https://pkg.go.dev/cmd/vet) |
| `staticcheck` | 补充 bug、性能、简化和风格静态分析；官方建议与 `go vet` 并行使用 | 不执行测试、race 或模块完整性验证 | 与少量 vet/编译诊断概念重叠，但规则集更广。[Staticcheck 官方说明](https://staticcheck.dev/docs/) |
| `errcheck` / `ineffassign` / `unused` | 未检查错误、无效赋值、未使用声明 | 不衡量行为覆盖或复杂度 | 属于 golangci-lint 默认集合；当前 4 条实际结果来自 `errcheck` |
| `go mod tidy -diff` | `go.mod` / `go.sum` 是否与源码所需依赖一致；`-diff` 不写文件且有差异时非零退出 | 不验证缓存内容、漏洞、许可证、是否为最新版本 | 与 `verify`、govulncheck 口径不同。[Go Modules Reference](https://go.dev/ref/mod#go-mod-tidy) |
| `go mod verify` | 已下载 module zip 与解压目录是否仍匹配校验值 | 不判断声明是否冗余，也不判断漏洞 | 与 `tidy` 互补。[Go Modules Reference](https://go.dev/ref/mod#go-mod-verify) |
| `govulncheck ./...` | 根据 Go 漏洞库与调用图识别标准库/依赖的已知可达漏洞 | 不保证发现未知漏洞；反射/unsafe 等会限制静态分析 | 不替代 `tidy` / `verify`；需要访问漏洞数据库 |
| `depguard` | 约束某目录能否 import 某 package，适合实现模块依赖方向 | 不验证 module 完整性或安全 | 应等目录/模块边界确定后配置。[golangci 配置](https://golangci-lint.run/docs/linters/configuration/#depguard) |
| `gomodguard_v2` / `gomoddirectives` | 直接 module allow/block/version 规则；限制本地 `replace` 等 go.mod 指令 | 不做调用可达漏洞分析 | 有明确供应链策略后再启用；旧 `gomodguard` 已弃用。[golangci 配置](https://golangci-lint.run/docs/linters/configuration/#gomodguard_v2) |

### 单元测试与集成测试在当前仓库中的边界

Go 的 `go test` 不按“单元/集成”语义自动分类；它按 package、文件名、测试函数和 build constraints 运行。当前 3 个测试都通过公开 `Service` 操作真实 SQLite 与 worker，因此完整门禁应全部运行。现在人为增加 `unit` / `integration` build tag 只会增加漏跑风险，没有带来执行成本收益。

若未来 OSM、HTTP 或招聘站点 adapter 引入真实网络/外部服务，再把可重复的本地集成测试留在默认 `go test ./...`，把需要凭证或不稳定网络的端到端检查单独命名；不要把所有 `_test.go` 都当作廉价单元测试。

## 四个复杂度 linter 的差异与当前测量

golangci-lint `v2.13.2` 实际捆绑 `cyclop v1.2.3`、`gocyclo v0.6.0`、`funlen v0.2.0`、`maintidx v1.0.0`，可在其[tagged go.mod](https://github.com/golangci/golangci-lint/blob/v2.13.2/go.mod)复核。

| 工具 | 指标 | 官方默认 | 当前生产最差 | 当前测试最差 | 默认结果 |
| --- | --- | --- | --- | --- | --- |
| `cyclop` | 函数圈复杂度；另可限制 package average | 函数最大 10；package average 默认关闭 | 9：`runWorker` | 18：重启恢复测试 | 仅测试失败 |
| `gocyclo` | 函数圈复杂度 | 30；golangci 文档建议 10–20 | 9：`runWorker` | 14：重启恢复测试 | 全部通过 |
| `funlen` | 函数源码行数与递归 statement 数 | 60 行 / 40 statements | 44 行：`completeWork`；21 statements：`claimWork` | 68 行 / 34 statements：重启恢复测试 | 仅测试因行数失败 |
| `maintidx` | 圈复杂度 + Halstead Volume + LOC 的综合可维护性指数；越高越好 | 报告 `<20` | 42：`claimWork`、`completeWork` | 35：重启恢复测试 | 全部通过 |

### `cyclop` 与 `gocyclo`

两者都从 1 开始，并为 `if`、循环、case/communication clause、`&&`、`||` 增加路径数。差异来自实现：

- [`gocyclo v0.6.0`](https://github.com/fzipp/gocyclo/blob/v0.6.0/complexity.go) 不计 `default` case / default communication clause；
- [`cyclop v1.2.3`](https://github.com/bkielbasa/cyclop/blob/v1.2.3/pkg/analyzer/analyzer.go) 计入所有 case / communication clause，并能计算 package average；
- 当前包含 `select default` 的测试因此显示 cyclop 18、gocyclo 14。两者并非“一个衡量圈复杂度、另一个衡量认知复杂度”。

候选建议是选 `cyclop`：它的默认 10 更接近当前规模，对 select 分支也更敏感。若团队更认可 gocyclo 的传统口径，也可选它并把阈值从默认 30 下调到 12 左右；不要两者同时作为硬门禁。

`cyclop.package-average` 建议初始保持 `0.0`（关闭）。从源码可推断，它按 package 内函数总复杂度 / 函数数计算，增加简单 helper 或拆 package 会改变分母，容易把架构整理误当成复杂度改善；函数级违规更直接可行动。

### `funlen`

[`funlen v0.2.0`](https://github.com/ultraware/funlen/blob/v0.2.0/funlen.go) 先递归计算 statements，未超限才检查源码行数；`ignore-comments` 只扣除函数内部注释行。多行 SQL、结构体字面量和显式错误处理会增加行数，却未必增加路径复杂度，所以不应只用 lines。

对 Job Atlas，`60 行 + 30 statements` 比单纯 60 行更合适：它保留 Go 的显式错误处理和可读 SQL 排版，同时在当前生产最大值 44 / 21 之上留出有限余量。

### `maintidx`

[`maintidx v1.0.0`](https://github.com/yagipy/maintidx/blob/v1.0.0/visitor.go) 使用 rebased maintainability index：圈复杂度、Halstead Volume、LOC 越高，指数越低；[项目 README](https://github.com/yagipy/maintidx/tree/v1.0.0#what-is-maintainability-index)明确提醒它是 experimental。

候选 `under: 40` 刚好位于当前生产最低 42 之下，可作为“接近退化边缘”的补充信号。但由于微小命名、字面量或排版变化也会影响 Halstead/LOC，建议先记录一轮 PR 数据；只有团队确认误报可接受后才升级为硬门禁。若必须第一天就硬门禁，可先用更宽的 `under: 35`。

## 候选配置片段

以下片段已用 golangci-lint `v2.13.2` 的 `config verify` 验证。只运行其中的复杂度检查时，当前仓库为 `0 issues`；若连同默认正确性检查一起启用，当前仍会被上述 4 个 `errcheck` 阻断，这是需要真实处理的基线欠账。

```yaml
version: "2"

run:
  relative-path-mode: gomod
  modules-download-mode: readonly

linters:
  # 显式列表避免 golangci-lint 升级时门禁集合悄悄变化。
  default: none
  enable:
    - errcheck
    - ineffassign
    - staticcheck
    - unused
    - cyclop
    - funlen
    # maintidx 先观察；确认后再加入硬门禁列表。

  settings:
    cyclop:
      max-complexity: 12
      package-average: 0.0
    funlen:
      lines: 60
      statements: 30
      ignore-comments: true
    maintidx:
      under: 40

  exclusions:
    generated: strict
    warn-unused: true
    rules:
      # 只豁免顶层测试场景；_test.go 中的 helper 仍受检查。
      - source: '^func Test'
        linters:
          - cyclop
          - funlen
          - maintidx

formatters:
  enable:
    - gofmt
  exclusions:
    generated: strict
```

golangci-lint 的 `generated: strict` 只排除符合 Go 官方标记 `// Code generated ... DO NOT EDIT.` 的真正代码生成文件。[配置说明](https://golangci-lint.run/docs/configuration/file/#linters-configuration) AI 写出的普通业务 `.go` 文件没有该标记，因此照常接受复杂度检查；不要为了绕过门禁给 AI 代码添加生成标记。

## 本地与 CI 的同一入口

### 完整门禁建议

`make check`（名称可由实施票决定）按下列语义顺序执行并在任一步失败时停止：

```text
1. golangci-lint fmt --diff
2. go mod tidy -diff
3. go mod verify
4. go test -count=1 ./...
5. go test -race -count=1 ./...
6. go vet ./...
7. golangci-lint run ./...
8. govulncheck ./...
```

如果保留第 6 步显式 `go vet`，候选 golangci 配置就不要启用 `govet`。`go test` 仍需保留，因为它只执行 vet 的高置信子集；`staticcheck` 也仍需保留，因为官方将其定位为与 `go vet` 并行的补充分析。

可提供 `make check-fast` 只运行格式、`tidy -diff`、普通测试、`go vet` 和 lint，缩短编辑反馈；完整 `make check` 仍包含 race 与 govulncheck。CI 的 required check 只调用 `make check`，这样本地复现不会依赖复制 workflow 内的隐含参数。

### 版本与网络边界

- Go 使用明确 patch 版本；当前 `go1.23.3` 已被 govulncheck 证明不适合作为门禁工具链。
- golangci-lint 使用固定二进制版本；官方不建议通过主项目 `go.mod` 直接安装它，而是建议二进制或隔离的工具 module。[本地安装说明](https://golangci-lint.run/docs/welcome/install/local/)
- govulncheck 也固定 CLI 版本，但它查询的是持续更新的 Go 漏洞数据库。完整门禁因此需要网络；网络不可用应报告“安全检查未执行”，不能当作“无漏洞”。
- 若项目暂时不希望每次本地完整检查访问网络，可保留 `make security`，但 CI 的 required target 必须组合为同一个 `make check-all`，并确保开发者能原样运行；不能让 CI 独自维护另一套命令。

## 逐步收紧策略

### 第 0 步：先清理真实阻塞

- 逐处处理 4 个 `errcheck`，只对语义上确实无需处理的返回值做局部、带理由的显式忽略；
- 选择无上述标准库可达漏洞的受支持 Go patch；
- 固定工具版本，保存完整基线命令输出。

### 第 1 步：建立不制造存量噪音的硬门禁

- 格式、`tidy -diff`、`verify`、普通测试、race、完整 vet、curated golangci linters、govulncheck 全部阻断合并；
- 复杂度使用一个圈复杂度工具（候选 cyclop 12）+ funlen 60/30；
- 仅豁免顶层 `func Test...`，测试 helper 仍检查；
- maintidx 40 先记录，不立刻阻断。

### 第 2 步：目录架构稳定后补模块边界

- 按确定后的业务深模块依赖方向配置 `depguard`；
- 若 ORM/数据库方案形成明确 allow/block 规则，再考虑 `gomodguard_v2`；
- 不在架构尚未决定时预先写一张宽泛依赖白名单。

### 第 3 步：用分布而不是感觉调阈值

- 每次改阈值都记录当时生产函数的 max cyclop、max statements、min maintidx；
- 阈值只能通过显式配置评审调整，禁止为单个新函数整体放宽；
- 单点例外使用精确 path/source/text 规则并写原因，同时开启 `warn-unused`，避免例外在代码消失后永久残留；
- package 拆分后重新测量，但不因指标自然下降就自动进一步收紧。

## 需要后续决策确认的四点

1. 圈复杂度最终选 `cyclop`（本文候选）还是 `gocyclo`；二者不并用。
2. `maintidx under 40` 是只观察，还是第一天就成为硬门禁；若立即硬门禁，是否先放宽到 35。
3. 是否接受仅对顶层 `func Test...` 豁免 cyclop/funlen/maintidx，而继续检查所有测试 helper。
4. 完整本地入口是否允许访问漏洞数据库；无论如何，CI required check 必须运行 govulncheck，并提供本地可复现的同名组合入口。

## 主要一手资料

- Go：[gofmt](https://pkg.go.dev/cmd/gofmt)、[`go test`](https://pkg.go.dev/cmd/go#hdr-Test_packages)、[Race Detector](https://go.dev/doc/articles/race_detector)、[`go vet`](https://pkg.go.dev/cmd/vet)、[Modules Reference](https://go.dev/ref/mod)、[govulncheck](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck)
- golangci-lint：[Quick Start](https://golangci-lint.run/docs/welcome/quick-start/)、[Configuration v2](https://golangci-lint.run/docs/configuration/file/)、[Linter settings](https://golangci-lint.run/docs/linters/configuration/)、[CI installation](https://golangci-lint.run/docs/welcome/install/ci/)
- 指标实现：[`cyclop v1.2.3`](https://github.com/bkielbasa/cyclop/blob/v1.2.3/pkg/analyzer/analyzer.go)、[`gocyclo v0.6.0`](https://github.com/fzipp/gocyclo/blob/v0.6.0/complexity.go)、[`funlen v0.2.0`](https://github.com/ultraware/funlen/blob/v0.2.0/funlen.go)、[`maintidx v1.0.0`](https://github.com/yagipy/maintidx/blob/v1.0.0/visitor.go)
- Staticcheck：[官方文档](https://staticcheck.dev/docs/)
