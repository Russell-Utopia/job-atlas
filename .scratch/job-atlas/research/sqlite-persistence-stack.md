# Job Atlas SQLite 数据访问与 schema 迁移方案评估

核验时间：2026-08-29。

本报告回答 GitHub 议题“评估 SQLite 数据访问与 schema 迁移方案”。它只使用当前仓库代码、仓库既有规格，以及 Go、SQLite、modernc SQLite、sqlc、GORM、Ent、goose、Atlas 的官方文档或官方源码。报告区分已核验事实、结合 Job Atlas 的推断和可供后续决策的建议，不替架构决策票作最终选型。

## 结论摘要

1. **当前真正需要保护的是事务语义，不是 CRUD 书写量。** `createRun` 要原子写入任务和全部工作项；`claimWork` 要按稳定顺序读取一个 `pending` 工作并以条件更新完成领取；`completeWork` 要原子完成工作项并在没有剩余工作时完成任务。任何数据访问方案都必须让这三类事务保持显式、可读、可做真实 SQLite 集成测试。
2. **ORM 不是硬性前置条件。** `database/sql`、sqlc、GORM、Ent 都能表达事务；但 GORM/Ent 的主要收益是实体 CRUD、关联和 schema 建模，而 Job Atlas 当前的核心是队列状态转换、覆盖判断和当前事实 upsert。换 ORM 不会消除特殊 SQL，反而可能同时保留 ORM 与 raw SQL 两套表达。
3. **当前最短的候选路线有两条：**
   - 继续使用 `database/sql + modernc.org/sqlite`，把手写 SQL 收进持久化 Adapter，并新增独立的 versioned migration 工具；
   - 使用 `sqlc + database/sql + modernc.org/sqlite` 生成查询与扫描代码，同时仍由独立工具负责 migration。
4. **若采用代码生成，sqlc 比 Ent 更贴近当前问题形状。** sqlc 以现有 SQL 为源并生成薄的 Go 调用层；Ent 会从 Go entity schema 生成 Client、Tx、每个实体的 CRUD builder、predicate 包、migrate 包和 hook 包，生成面明显更大。[sqlc SQLite 入门](https://docs.sqlc.dev/en/latest/tutorials/getting-started-sqlite.html)、[Ent code generation](https://entgo.io/docs/code-gen/)
5. **当前内嵌 `CREATE TABLE IF NOT EXISTS` 不能继续承担 schema 演进。** 它没有版本历史，无法表达数据搬迁，也不会发现已有表与期望 schema 不一致。新增重试记录、当前岗位事实等表之前，应先建立可回放、可验证的 versioned migrations。
6. **goose 是当前规模下更小的迁移候选，Atlas 是能力更完整但更重的候选。** goose 可以直接使用现有 `*sql.DB`、`modernc.org/sqlite` 和 `embed.FS`，每个 SQL migration 默认运行在一个事务中；Atlas 能从目标 schema 生成 SQLite migration、维护 checksum、验证和 lint migration，但引入额外 CLI/config 工作流。[goose Provider](https://pressly.github.io/goose/documentation/provider/)、[goose SQL annotations](https://pressly.github.io/goose/documentation/annotations/)、[Atlas versioned workflow](https://atlasgo.io/guides/evaluation/setup-migrations)
7. **无论选哪一层，都要先固定 SQLite 连接契约。** `foreign_keys` 与 `busy_timeout` 是连接级设置；当前 `SetMaxOpenConns(1)` 使初始化 PRAGMA 恰好作用于唯一连接，但以后只要扩大连接池，就应由 modernc DSN 的 `_pragma` 为每个新连接设置，而不是只在启动时对 `sql.DB.Exec` 一次。[SQLite foreign keys](https://www.sqlite.org/foreignkeys.html)、[SQLite PRAGMA](https://www.sqlite.org/pragma.html)、[modernc SQLite driver](https://pkg.go.dev/modernc.org/sqlite@v1.38.2)

## 1. 当前仓库的持久化形状

### 1.1 已核验事实

当前仓库只有一个 SQLite 持久化实现：

- `sqlite_store.go` 使用 `database/sql` 和 `modernc.org/sqlite v1.38.2`；`go.mod` 的 Go 版本是 1.23。
- `discovery_runs` 保存对外任务的当前状态与顶层错误。
- `discovery_work_items` 保存“一个任务 × 一个来源 × 一个城市”的覆盖工作及 `pending / running / completed` 状态。
- 初始化阶段设置 `foreign_keys=ON`、`journal_mode=WAL`、`busy_timeout=5000`，然后执行内嵌 schema。
- `sql.DB` 被限制为 `SetMaxOpenConns(1)`。
- `createRun`、`claimWork`、`completeWork` 都通过 `sql.Tx` 显式提交或回滚。
- `claimWork` 先按 `created_at / run_id / city_position / source_position` 找一项 `pending` 工作，再以 `... AND status = 'pending'` 条件更新，并要求 `RowsAffected() == 1`。
- 测试使用 `t.TempDir()` 中的真实 SQLite 文件，覆盖空结果完成、进程内关闭后恢复未完成工作、恢复时缺少原来源三种行为；没有把存储替换成 mock。

仓库的 Interface 规格还要求后续持久化：扫描期间已确认的当前岗位、来源工作检查点、失败与重试执行记录，以及 `RestartDiscovery` 对未完成工作重新激活。ADR 要求相同来源岗位 ID 更新当前事实，不保存旧页面或旧 JD。由此可知后续查询仍会以显式的状态转换、条件更新、幂等 upsert 和事务内一致性判断为主，而不只是单表 CRUD。

### 1.2 项目推断

- `SetMaxOpenConns(1)` 让同一进程中的所有数据库操作串行使用一个连接，所以当前启动 PRAGMA 不会漏到第二条池连接，且单 worker 不会在进程内竞争工作领取。
- 它不阻止第二个 Job Atlas 进程打开同一个数据库文件。SQLite 允许多个读事务，但同一时刻只允许一个写事务；默认 `BEGIN` 是 `DEFERRED`，读后升级为写事务时可能返回 `SQLITE_BUSY`。[SQLite transaction](https://www.sqlite.org/lang_transaction.html)
- 当前条件更新和 `RowsAffected` 是必要保护，但在未来多进程或多 writer 场景下，还必须明确 writer 数量、`BEGIN IMMEDIATE`/重试策略以及启动 migration 的互斥规则，不能靠 ORM 默认值推断安全性。

## 2. 数据访问方案对比

| 方案 | 事务与工作领取 | SQLite / modernc 适配 | 生成代码影响 | migration 职责 | 测试与复杂度 | 对当前形状的含义 |
| --- | --- | --- | --- | --- | --- | --- |
| 手写 `database/sql` | `sql.Tx` 直接表达；SQL、条件更新和 `RowsAffected` 全部可见 | 已经运行；无额外适配层 | 无生成代码；扫描、nullable 和参数类型由人维护 | 必须另选工具 | 真实 SQLite 测试直接；依赖最少，但 schema/query 错配主要在运行时暴露 | 查询数量少时最小；查询增多后重复扫描和样板代码会增长 |
| sqlc | 生成 `WithTx(*sql.Tx)`；`:execrows` 可直接返回受影响行数 | Go + SQLite 当前标为 Beta；默认生成 `database/sql` 调用 | 每个 query/schema 生成 models、DBTX、query methods；应固定版本并校验生成漂移 | sqlc 明确不执行 migration，可解析 goose/Atlas 等 migration 目录 | SQL 仍可审查，参数与扫描生成；SQLite 暂无 database-backed analyzer，复杂 SQL仍需真实 DB 测试 | 对显式 SQL/事务最贴合，代价是新增 generator 与 generated package |
| GORM | `Transaction(func(*gorm.DB))`、手动 Tx、savepoint；特殊领取可用 raw SQL | 官方 SQLite driver 默认基于 `go-sqlite3`，需要 CGO；保留 modernc 需现有连接/自定义 driver 或社区纯 Go driver | 核心反射式 ORM 不要求生成；若用 Gen 又增加生成面 | `AutoMigrate` 或独立 versioned tool；官方说明复杂阶段可转 Atlas | 有 DryRun，但真实锁/事务仍须 SQLite 集成测试；模型 tags、callbacks、raw SQL 可能并存 | 当前关联 CRUD 很少，ORM 收益有限；纯 Go driver 路线需额外决策 |
| Ent | `Client.Tx` / transactional client；raw SQL 需启用 escape hatch | 支持 SQLite，并可把自建 `*sql.DB` 包装为 Ent driver；官方示例默认仍用 `go-sqlite3` | 生成 Client、Tx、entity、每实体 CRUD builder、predicate、migrate、hook 等多个包 | 自动 migration 或 Atlas versioned migration | builder 类型安全；生成量和 schema 层最大，特殊队列 SQL可能仍需 modifier/raw SQL | 若未来核心变成大量关联实体与统一 schema policy 才可能回收成本；当前偏重 |

### 2.1 `database/sql` 手写 SQL

**官方事实：**

- `sql.DB` 是连接池；事务通过 `DB.BeginTx` 获得 `sql.Tx`，事务内应只使用该 `Tx` 的查询和执行方法，不能混用事务外的 `sql.DB`。[Go database handle](https://go.dev/doc/database/open-handle)、[Go transactions](https://go.dev/doc/database/execute-transactions)
- `sql.Tx` 把一组读取和更新固定在同一连接并提供 `Commit` / `Rollback`，这与当前三个原子方法的写法一致。

**项目推断：**

- 这是依赖和生成面最小的方案，也是表达 SQLite 专属 CTE、`RETURNING`、条件更新和 PRAGMA 最直接的方案。
- 代价不在事务能力，而在查询数量增加后，SQL 字符串、参数结构、`Scan` 顺序、nullable 转换和错误包装全部手工维护。schema 改列后，Go 编译器不会发现旧 `Scan`，必须依靠 migration 回放和真实数据库测试。
- 若保留此方案，应把存储实现放在明确的 SQLite Adapter 内，不要把 `*sql.DB` 或表结构类型暴露进业务深模块；业务模块只看到以领域行为命名的小接口。

### 2.2 sqlc

**官方事实：**

- sqlc 读取 schema 与命名 SQL query，并输出可读的 Go models、`DBTX` 与 query methods。[sqlc SQLite 入门](https://docs.sqlc.dev/en/latest/tutorials/getting-started-sqlite.html)
- 生成的 `Queries.WithTx(tx)` 可以把同一组 query 绑定到 `*sql.Tx`；因此采用 sqlc 不等于放弃显式事务边界。[sqlc transactions](https://docs.sqlc.dev/en/latest/howto/transactions.html)
- `:execrows` 生成返回受影响行数的方法，可保留当前“条件状态转换必须恰好更新一行”的检查。[sqlc query annotations](https://docs.sqlc.dev/en/latest/reference/query-annotations.html)
- sqlc **不执行 database migration**，但能按顺序解析 Atlas、goose、golang-migrate 等 migration 目录并忽略 down migration。[sqlc DDL and migrations](https://docs.sqlc.dev/en/latest/howto/ddl.html)
- 截至当前官方 1.31.1 文档，Go 对 SQLite 的支持等级是 Beta；增强的 database-backed analyzer 尚不支持 SQLite。[sqlc language support](https://docs.sqlc.dev/en/stable/reference/language-support.html)、[sqlc generate](https://docs.sqlc.dev/en/latest/howto/generate.html)

**项目推断：**

- sqlc 消除的是参数/扫描样板和一部分 schema-query 错配，不会设计事务，也不会替代 migration。
- `claimWork` 仍应是一个有业务名字的持久化方法；方法内部可以 `BeginTx` 后使用 `queries.WithTx(tx)`，或者在 SQLite 版本与 sqlc parser 实测支持后，把候选选择和状态更新收敛为单条 `UPDATE ... RETURNING`。不能为了生成器能解析而弱化原子语义。
- 生成代码应放在内部 Adapter 包；业务层不要依赖 sqlc 自动生成的全量 `Querier`。否则表级 CRUD 会泄漏成模块 Interface。
- 质量门禁应固定 sqlc 版本，并运行 `sqlc generate` 后检查工作树无差异。生成文件可以从“人工代码复杂度阈值”中排除，但必须编译、测试且禁止手工编辑。

### 2.3 GORM

**官方事实：**

- GORM 为 create/update/delete 默认增加事务，也支持显式 transaction callback、手动 Tx、nested transaction 和 savepoint。[GORM transactions](https://gorm.io/docs/transactions.html)
- 它允许 raw SQL、`RowsAffected` 和 DryRun，因此理论上可以保留特殊领取 SQL。[GORM SQL builder](https://gorm.io/docs/sql_builder.html)
- 官方 SQLite Dialector 源码明确忽略 `FOR UPDATE` 等 row-level locking clause，因为 SQLite 不支持行锁。[GORM SQLite Dialector source](https://github.com/go-gorm/sqlite/blob/master/sqlite.go)
- GORM 官方 SQLite driver 默认基于 `github.com/mattn/go-sqlite3`，需要 CGO；官方 README 把纯 Go driver 列为 community alternatives。Dialector 源码允许传入已有 `gorm.ConnPool`，但这仍需专门验证 modernc 与 Dialector 的组合，而不是默认支持路径。[GORM SQLite driver](https://github.com/go-gorm/sqlite)、[GORM SQLite Dialector source](https://github.com/go-gorm/sqlite/blob/master/sqlite.go)
- `AutoMigrate` 会创建缺少的表、外键、约束、列和索引，能修改部分列属性，但不会删除未使用列；对 SQLite 不支持的变更，Migrator 会建新表、复制数据、删旧表、再重命名。官方还给出转向 Atlas versioned migrations 的路径。[GORM migration](https://gorm.io/docs/migration.html)

**项目推断：**

- GORM 能做这件事，但当前仓库不靠复杂关联、preload 或通用 CRUD 获益；核心 query 最终仍可能写 raw SQL。这样会同时承担 model tags、GORM callbacks/默认事务和 SQLite 特殊 SQL三套概念。
- 从现有纯 Go `modernc.org/sqlite` 切到 GORM 默认 SQLite driver 会改变构建条件；保留纯 Go 又要引入社区 Dialector 或非默认 Conn 组合。这是实际迁移成本，不能只写成 `gorm.Open(sqlite.Open(...))`。
- `AutoMigrate` 适合原型启动，不适合取代 Job Atlas 的审计型版本历史：它不能单凭最终 struct 说明某次数据如何搬迁，也不会删除旧列。若最终选 GORM，仍应独立决定 versioned migration。

### 2.4 Ent

**官方事实：**

- Ent 从 Go schema 生成 Client、Tx、各实体 CRUD builder、entity、predicate、migrate、hook 等资产，并要求 generator 与运行库版本一致。[Ent code generation](https://entgo.io/docs/code-gen/)
- Ent 支持显式 `Client.Tx` 和 transactional client；提交后的 entity 若继续沿 edge 查询还需要 `Unwrap`。[Ent transactions](https://entgo.io/docs/transactions/)
- Ent 可从已有 `*sql.DB` 创建 SQL dialect driver，因而存在保留 modernc 驱动的接入点。[Ent `sql.DB` integration](https://entgo.io/docs/sql-integration/)
- `sql/execquery` feature 可执行底层 raw SQL，但官方提醒这些语句会绕过 Ent hooks、privacy 和 validators。[Ent feature flags](https://entgo.io/docs/feature-flags/)
- Ent 支持 runtime automatic migrations，也支持由 Atlas 管理 SQL 文件的 versioned migrations；官方说明简单项目常从 automatic 开始，需要更多控制时转向 versioned。[Ent migration flows](https://entgo.io/docs/versioned/intro/)

**项目推断：**

- Ent 的生成模型适合实体和关系是主要复杂度的系统。Job Atlas 当前较难的是持久化状态机和工作领取，不是对象图遍历。
- 如果 `claimWork` 需要 raw SQL escape hatch，就要额外保证这条路径绕过 hooks/validator 不会破坏统一策略；这降低了采用 Ent 的直接收益。
- Ent schema 与实际 SQL migration 之间还要建立生成和 diff 工作流。相较 sqlc，它不仅生成 query 调用层，还会成为 schema 和实体 API 的主要权威，属于更大范围的架构承诺。

## 3. SQLite 运行约束先于 ORM 选择

### 3.1 连接级 PRAGMA

**官方事实：**

- `sql.DB` 是连接池，调用 `Exec` 时会临时取得一条连接。[Go connection management](https://go.dev/doc/database/manage-connections)
- SQLite `foreign_keys` 默认关闭，并且必须为每条数据库连接单独启用；`busy_timeout` 设置该连接的 busy handler。[SQLite foreign keys](https://www.sqlite.org/foreignkeys.html)、[SQLite PRAGMA](https://www.sqlite.org/pragma.html)
- `journal_mode=WAL` 写入数据库后会跨连接、跨重开保持，但它不使 SQLite 允许多个并发 writer；SQLite 同时只能有一个 write transaction。[SQLite PRAGMA](https://www.sqlite.org/pragma.html)、[SQLite transaction](https://www.sqlite.org/lang_transaction.html)
- 当前使用的 modernc v1.38.2 支持 DSN `_pragma`，每次打开连接时执行给定 PRAGMA；也支持 `_txlock=deferred|immediate|exclusive`。[modernc v1.38.2 docs](https://pkg.go.dev/modernc.org/sqlite@v1.38.2)、[modernc v1.38.2 source](https://gitlab.com/cznic/sqlite/-/blob/v1.38.2/sqlite.go)

**建议作为后续决策约束：**

- 保留单 writer / `MaxOpenConns(1)` 作为 v1 默认，除非并发基准证明它是瓶颈。
- 把 `foreign_keys`、`busy_timeout` 等连接设置移到受信任的 modernc DSN 或 connection hook，并在每次打开数据库时查询回读，不能仅依赖一次 `db.Exec`。
- `journal_mode=WAL` 可在受控初始化或 migration 前确认；必须检查返回的实际 mode，因为设置 PRAGMA 会返回最终 mode。
- 任何扩大连接池、拆读写池或允许多进程的改动，都要新增锁竞争、busy timeout 和重复领取测试。

### 3.2 `DEFERRED` 与工作领取

**官方事实：**

- SQLite 默认 `BEGIN DEFERRED`；若事务先读再写，它在首次写入时尝试升级为 write transaction，其他连接已写入时可能得到 `SQLITE_BUSY`。`BEGIN IMMEDIATE` 会在事务开始时取得写事务，若已有 writer 则开始阶段就失败/等待 busy handler。[SQLite transaction](https://www.sqlite.org/lang_transaction.html)
- modernc v1.38.2 的 `_txlock` 决定非 read-only `BeginTx` 使用普通 `BEGIN`、`BEGIN IMMEDIATE` 或 `BEGIN EXCLUSIVE`；源码没有把 Go 的其他 isolation level 映射成 SQLite isolation mode。[modernc v1.38.2 source](https://gitlab.com/cznic/sqlite/-/blob/v1.38.2/sqlite.go)

**建议作为后续验证项：**

- 在仍为单连接、单 worker 时，当前领取事务已被进程内串行化，不必仅为理论并发重写。
- 若计划多 worker 或多进程，应对两种候选做真实并发测试：
  1. 单 writer handle + `BEGIN IMMEDIATE` + 现有条件更新；
  2. 单条条件 `UPDATE ... RETURNING` 完成选择与领取。
- 验收不是“没有报错”，而是：每个工作项至多被领取一次；锁竞争在声明的 timeout/retry 内结束；进程崩溃后的 `running -> pending` 恢复仍成立。

### 3.3 SQLite schema 变更

当前锁定的 `modernc.org/sqlite v1.38.2` 属于把内核升级到 SQLite 3.50.1 的 v1.38 系列，而不是当前 SQLite 文档所描述的更新内核版本。[modernc changelog](https://gitlab.com/cznic/sqlite/-/blob/master/CHANGELOG.md) 评估 migration SQL 时应以仓库实际锁定的 SQLite 版本为准，不能使用新版本后来增加的 DDL 能力。

SQLite 3.50 的原生 `ALTER TABLE` 主要支持 rename table、rename column、add column、drop column；修改约束、主键或其他表定义通常需要“建新表 → 复制数据 → 删旧表 → 重命名 → 重建索引/触发器/视图 → foreign key check”的受控过程。[SQLite ALTER TABLE](https://www.sqlite.org/lang_altertable.html)

这意味着 migration 方案至少要能：

- 按版本严格排序并记录已应用版本；
- 在临时数据库从空库完整回放；
- 从上一版本逐步升级并验证数据搬迁；
- 对重建表后的外键、索引和约束运行检查；
- 失败时停止服务启动，不能让 worker 在半迁移 schema 上运行。

## 4. schema migration 方案对比

| 方案 | 版本与回放 | SQLite / modernc | 事务与复杂变更 | 工具成本 | 适用判断 |
| --- | --- | --- | --- | --- | --- |
| 当前 `CREATE IF NOT EXISTS` | 无版本；只能重复创建缺少对象 | 已运行 | 不表达数据迁移，不能发现已有对象形状错误 | 最低 | 只能作为一次性 bootstrap，不能继续扩展 |
| 自建 `PRAGMA user_version` runner | 应用自行定义版本整数、排序、校验与失败语义 | SQLite 原生；SQLite 不解释该值 | 全部事务与重建表逻辑自行实现 | 表面少依赖，实际维护职责最大 | 除非迁移永远极少且愿意拥有 runner，否则不值得自建 |
| goose | timestamp/sequential migration；数据库版本表；SQL 或 Go migration | CLI 官方列出 `modernc.org/sqlite`；Provider 接收现有 `*sql.DB` 和 `fs.FS` | 单个 SQL 文件默认事务；可显式 no-transaction；默认不替应用做跨实例锁 | 一个 Go library/CLI，文件格式简单 | 当前小型单进程服务的低成本候选 |
| Atlas versioned | migration directory + `atlas.sum`；支持 diff、validate、lint、apply | 官方支持 SQLite 与内存 dev database | 可生成 SQLite 重建表计划并在 dev DB 验证；应用方式需单独集成 | 独立 CLI、`atlas.hcl` 与 checksum 工作流 | 需要自动 diff、严格 schema review、ORM schema 集成时更有价值 |
| GORM/Ent runtime auto migration | 根据运行时 model/schema 计算目标状态 | 两者支持 SQLite 特殊变更 | 由框架生成变更；历史意图和数据搬迁不如显式 SQL 清楚 | 与 ORM 绑定 | 可用于测试/原型，不建议成为本项目唯一生产迁移记录 |

### 4.1 goose

**官方事实：**

- goose 管理增量 SQL changes 或 Go functions；Provider 直接接收 dialect、`*sql.DB`、`fs.FS`，因此可以把 migration 嵌入 Job Atlas 二进制。[goose overview](https://pressly.github.io/goose/)、[goose Provider](https://pressly.github.io/goose/documentation/provider/)
- 官方 CLI 的 SQLite driver 是 `modernc.org/sqlite`。[goose commands and supported drivers](https://pressly.github.io/goose/documentation/cli-commands/)
- 一个 SQL migration 文件内的语句默认在同一事务执行；必须在事务外运行时可加 `-- +goose NO TRANSACTION`。[goose annotations](https://pressly.github.io/goose/documentation/annotations/)
- Provider 默认不做 migration session lock，由调用方保证不会有多个实例同时迁移。[goose Provider](https://pressly.github.io/goose/documentation/provider/)

**项目推断：**

- 对当前单进程、本地 SQLite 服务，goose 带来的概念最少：保留 SQL 为权威，服务启动时先 `Up`，成功后才启动 worker。
- 如果未来允许同一数据库由多个进程启动，必须另设文件锁/部署互斥或单独 migrate 命令；不能误以为 goose 默认锁住 SQLite migration。
- sqlc 官方可解析 goose migrations，且 goose 官方也给出 sqlc + SQLite 的组合示例；两者职责清楚分离。[goose + sqlc](https://pressly.github.io/goose/blog/2024/goose-sqlc/)

### 4.2 Atlas

**官方事实：**

- Atlas 支持 declarative 和 versioned 两种工作流；versioned migration 目录包含 SQL 文件与 `atlas.sum` integrity file。[Atlas migration workflow](https://atlasgo.io/guides/evaluation/setup-migrations)
- `migrate diff` 会读取目标 schema、回放 migration 目录到 dev database、计算 diff并写 migration；SQLite dev database 可以是内存数据库。[Atlas SQLite](https://atlasgo.io/getting-started/sqlite-declarative-sql)
- `migrate validate` 能校验目录 checksum；提供 dev URL 时还会按顺序执行 migration 来验证 SQL 语义。[Atlas CLI reference](https://atlasgo.io/cli-reference)
- Atlas 是 GORM 和 Ent 官方文档给出的 versioned migration 集成方向。[GORM migration](https://gorm.io/docs/migration.html)、[Ent versioned migrations](https://entgo.io/docs/versioned-migrations/)

**项目推断：**

- Atlas 的优势是从期望 schema 生成并检查迁移，而不仅是执行手写文件；在采用 Ent/GORM schema 或表结构频繁变化时，这会减少手工编写 SQLite 重建表流程的风险。
- 对当前两张表，额外 CLI、HCL、dev database 和 checksum 冲突处理可能比实际 schema 复杂度更大。若没有明确采用 schema-as-code / auto-diff 的需求，goose 更容易建立最小可靠基线。

## 5. 测试策略不应随工具退化

不论最终选哪个数据访问方案，都建议保留当前“真实 SQLite 文件 + 真实状态/恢复”的测试接缝，原因是 mock 无法验证 SQLite 锁、PRAGMA、DDL、`RowsAffected` 和崩溃恢复语义。

最低测试矩阵：

1. **migration 从空库回放**：全部版本执行成功，最终 `PRAGMA foreign_key_check` 无结果，关键 index 存在。
2. **逐版本升级**：至少保留一个上一版本 fixture，升级后当前任务、工作项与岗位事实不丢失。
3. **重复启动**：已到最新版本再次启动不改数据。
4. **失败 migration**：故意失败时版本不前进，worker/HTTP 服务不启动。
5. **领取竞争**：按最终支持的 worker/进程数量并发领取，没有重复工作；busy timeout/retry 行为符合声明。
6. **恢复**：进程中断留下的 `running` 工作在下一次启动回到可领取状态，已经完成的工作不重做。
7. **生成漂移**（仅 sqlc/Ent）：运行固定版本 generator 后 `git diff --exit-code`；生成文件参与编译与真实 SQLite 集成测试。

GORM 的 DryRun 可以检查生成 SQL 形状，但官方文档明确它只生成 SQL 与参数，不执行；它不能代替真实 SQLite 锁和 migration 测试。[GORM SQL builder](https://gorm.io/docs/sql_builder.html)

## 6. 可供架构决策票采用的建议

以下是基于当前事实的条件建议，不是最终选型：

### 建议 A：先把 migration 从数据访问选型中拆开

无论保留手写 SQL、引入 sqlc，还是选择 ORM，都应先决定“所有 schema 变化使用可回放的 versioned migration”。不要让 `AutoMigrate` 或 `CREATE IF NOT EXISTS` 成为唯一生产路径。

在当前规模下，可先把 **goose** 作为最小候选，把 **Atlas** 保留为需要 schema diff/lint 或采用 Ent/GORM schema 时的增强候选。最终选择应明确：migration 是随应用启动、由单独命令执行，还是由部署阶段执行；v1 单机 SQLite 最简单的是“启动 worker 前、同一进程、失败即退出”，但必须声明不允许多个实例同时迁移。

### 建议 B：数据访问优先在两条小路线中决策

1. **若近期持久化 query 仍少、团队更看重零生成与完全显式：** 保留 `database/sql`，把 SQL 按持久化行为拆小并加强 migration/集成测试。
2. **若岗位事实、执行记录、检查点加入后 query/Scan 样板明显增长，且能接受固定 generator：** 对一个代表性事务做 sqlc spike；只有在 SQLite parser 能无损表达 `claimWork`、upsert 和 migration schema 后，再采用 sqlc。

GORM 与 Ent 不应仅因为“Go 项目通常要 ORM”而入选。只有当后续表关系和通用 CRUD 成为主要复杂度、且团队愿意接受其 driver/schema/codegen 工作流时，再重新比较它们。

### 建议 C：用一个小型验证票消除剩余技术不确定性

在最终决策前，可做不进入业务主线的最小 spike，同时验证：

- modernc v1.38.2 下，`_pragma=foreign_keys(1)`、`_pragma=busy_timeout(5000)` 和候选 `_txlock=immediate` 对每条连接生效；
- 两个并发领取者在目标配置下不会重复领取，锁等待与错误可控；
- sqlc 当前版本能解析代表性的“条件领取 + 返回工作项”、当前岗位 upsert、nullable error 和 goose migration 目录；
- `sqlc generate` 实际增加多少生成文件/行数，生成代码能否完全留在 SQLite Adapter 内。

若 spike 失败，应回到手写 `database/sql + goose`，而不是为了迁就生成器改变业务原子性。

## 7. 最终架构决策需要明确的字段

后续关闭真正的选型票时，应明确记录：

- 数据访问：手写 `database/sql`、sqlc、GORM 或 Ent 中哪一个是仓库级默认；是否允许 raw SQL escape hatch。
- SQLite driver：是否继续锁定 `modernc.org/sqlite` 纯 Go 路线。
- 连接契约：DSN PRAGMA、最大 writer connection 数、是否允许多进程、transaction begin mode、busy retry 归属。
- migration：goose / Atlas / 其他；migration 文件位置；启动时还是独立命令应用；并发迁移互斥方式。
- 生成代码：generator 版本固定方式、输出包、是否提交、生成漂移门禁、lint 对生成目录的处理。
- 测试：空库回放、旧库升级、外键检查、竞争领取、崩溃恢复的统一命令。

只有这些字段一起落定，“选了 ORM/SQL 工具”才会变成可执行的 Go 工程架构基线。
