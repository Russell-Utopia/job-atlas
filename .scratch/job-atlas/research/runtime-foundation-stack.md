# HTTP、配置、日志与运行时基础栈研究

核验时间：2026-08-29。

本报告服务于 [“评估 HTTP、配置、日志与运行时基础栈”](https://github.com/Russell-Utopia/job-atlas/issues/27)。它只使用 Go 官方文档/源码入口与候选库维护者的官方文档，提供事实、对 Job Atlas 的推断和候选建议；最终选择仍由后续架构决策票确认。

## 1. 已知约束与结论摘要

### 1.1 仓库事实

- `go.mod` 当前声明 Go 1.23；因此本报告只把 Go 1.23 已具备的标准库能力当作可直接采用的基线。
- v1 是独立 HTTP/JSON 常驻进程，对外只有 `StartDiscovery / GetDiscovery / RestartDiscovery`，不提供 Web UI、取消、分页或游标。
- 发现任务比单次 HTTP 请求长。请求只创建、查询或重新激活持久化任务；客户端断开不能取消已经创建的发现任务。
- 当前 `Service` 已自行启动一个后台 worker，并用 `context.CancelFunc + sync.WaitGroup` 在 `Close()` 中停止和等待；SQLite 会在下次 `Open()` 时把中断的 `running` 工作恢复为 `pending`。
- 当前 worker 遇到 `claimWork` 或 `completeWork` 错误会直接退出，但没有把致命错误暴露给进程运行时。这是运行时 Interface 缺口，不是换路由库能解决的问题。

### 1.2 低风险候选方向

| 领域 | 标准库候选 | 第三方候选何时才产生净收益 | 当前判断依据 |
| --- | --- | --- | --- |
| HTTP 路由 | Go 1.23 `net/http.ServeMux` | 路由分组、子路由挂载和大型中间件栈成为真实需求时再评估 `chi` | v1 只有三个方法+路径路由，Go 1.22 起标准路由已支持方法和通配符 |
| JSON | `encoding/json` + 严格解码辅助函数 | 出现已测量的编解码瓶颈或必须使用另一套 JSON 语义时 | 当前 payload 是小型命令与完整快照，没有多协议需求 |
| 配置 | `flag` + `os.LookupEnv`，一次加载为强类型值 | 必须合并多种配置文件、远程 KV、热更新时再评估 Viper | v1 配置项少，而且启用来源影响任务完成语义，不适合运行中隐式变化 |
| 日志 | `log/slog` | 日志成为已测量热点，且 Zap 的编码/生态能力有明确需求时 | Go 1.23 已有结构化级别、属性、JSON/Text Handler 和 Handler Seam |
| 进程与 worker | `os/signal`、`context`、`sync.WaitGroup` | 多个并发组件都需“首错取消+错误汇总”时评估 `x/sync/errgroup` | 当前真正缺的是错误上报和可等待生命周期，而不是调度框架 |

这张表不是最终选型。它表明：**以当前范围为证据，标准库优先可以覆盖 v1；第三方库应由新增能力或测量结果触发，而不是作为“Go 服务默认全家桶”一次性引入。**

## 2. HTTP 路由与 Handler 边界

### 事实

Go 1.22 已为 `net/http.ServeMux` 增加 HTTP 方法和路径通配符匹配，例如 `POST /items` 与 `GET /items/{id}`，路径值通过 `Request.PathValue` 取得；Go 官方说明这让许多项目可以少一个路由依赖，同时仍承认高级路由需求可以使用第三方框架。[Go 1.22 routing enhancements](https://go.dev/blog/routing-enhancements)、[Go 1.22 release notes](https://go.dev/doc/go1.22#enhanced_routing_patterns)、[Go 1.23 `net/http.ServeMux`](https://pkg.go.dev/net/http@go1.23.0#ServeMux)

`chi` 仍然保持 `net/http` Handler 兼容，并提供路由分组、子路由挂载、路由遍历/文档以及 Request ID、Recoverer、节流等可选中间件；其核心路由包官方声明没有外部依赖。[chi official README](https://github.com/go-chi/chi#readme)

### 对 Job Atlas 的推断

三个操作可以直接表达为三个方法+路径模式；即使最终 URL 命名还未确认，它们也只需要一个集合路由和一个 `{runID}` 路径参数。当前没有嵌套资源树、API 版本并存、按租户挂载子路由或大量分组中间件。

HTTP Adapter 应只负责：

1. 把请求解码为调用参数；
2. 调用应用 Interface；
3. 把领域返回值/已知错误映射为稳定 HTTP 状态与 JSON；
4. 记录请求边界事实。

它不应启动 worker、不应理解 OSM/ATS，也不应让 `http.Request.Context()` 成为发现任务的生命周期。请求 context 只覆盖“把启动命令可靠写入 SQLite”这次调用；持久化成功后的扫描由进程级 worker context 管理。

### Interface 与维护成本

- `ServeMux`：没有新增模块依赖；需要仓库自己维护少量 JSON 错误映射、请求日志和 panic recovery 中间件。
- `chi`：路由仍能暴露为 `http.Handler`，迁移成本不高；但会增加一个版本化依赖，并容易顺手引入当前不需要的中间件和框架约定。
- 无论使用哪个路由器，都应让组装结果是 `http.Handler`，而不是让业务模块依赖具体路由类型。这样测试可以直接使用 `httptest`，路由选择也不进入业务 Interface。

### 当前不需要

- REST 框架自定义 Context、依赖注入容器、控制器基类；
- gRPC/GraphQL、路由文档生成、CORS、压缩、限流和认证全家桶；
- WebSocket、SSE 或流式响应。当前规格的 `GetDiscovery` 返回完整快照。

若服务默认只监听 loopback，TLS/认证可以暂不放进 v1 进程；**若允许监听非 loopback 地址，访问控制和传输保护会立刻成为另一项必须显式解决的安全边界**，不能因“没有 Web UI”而忽略。

### 候选建议

以“只有三个路由”为前提，先用 `http.NewServeMux`；只有路由规模或中间件组合复杂度在实现中真实增长，才重新比较 `chi`。这不是最终选型，而是一条可验证的引入阈值。

## 3. JSON 编解码与请求边界

### 事实

Go 1.23 的 `encoding/json.Decoder` 可以从流读取 JSON，`DisallowUnknownFields` 可在目标结构体不存在字段时返回错误；`Token` 在输入结束时返回 `io.EOF`，因此可以用第二次读取确认请求中没有多余 JSON 值。[Go 1.23 `encoding/json.Decoder`](https://pkg.go.dev/encoding/json@go1.23.0#Decoder)、[`DisallowUnknownFields`](https://pkg.go.dev/encoding/json@go1.23.0#Decoder.DisallowUnknownFields)

`http.MaxBytesReader` 专门限制请求体大小，并在超限读取时返回 `MaxBytesError`，用于避免异常或恶意大请求耗尽服务资源。[Go 1.23 `http.MaxBytesReader`](https://pkg.go.dev/net/http@go1.23.0#MaxBytesReader)

### 对 Job Atlas 的推断

`StartDiscovery` 的输入只有城市数组，`RestartDiscovery` 很可能无需 JSON body，输出结构已有明确字段。因此标准库足以完成类型化编解码。真正需要统一的不是 JSON 框架，而是一个很薄的 Adapter 内约定：

- 仅接受预期的 JSON media type；
- 对有 body 的命令设置小而明确的字节上限；
- 拒绝未知字段和第一个 JSON 值之后的额外内容；
- 空城市、未知 `runID`、冲突/不可恢复错误与内部错误使用一致的 JSON 错误形状；
- 先设置 `Content-Type: application/json`，再写状态与响应；不要把内部错误文本原样暴露给调用方。

`GetDiscovery.jobs` 当前没有分页，且会返回不断增长的完整岗位快照。请求 body 可以很小，但响应大小不一定小；后续选择 `WriteTimeout` 和测试数据规模时必须体现这个既有 Interface 约束。

### Interface 与维护成本

建议只在 HTTP Adapter 内维护 `decodeJSON`、`writeJSON`、`writeError` 这类未导出帮助函数；不要先抽象一个通用 `JSONCodec` Interface。这样严格解码规则只有一个位置，业务层继续接收 Go 值。

引入第三方 JSON 包会新增标签/错误语义和版本维护，但当前没有性能数据或协议能力证明其必要性。若未来基准显示完整岗位快照编码成为瓶颈，应拿真实 `Discovery` payload 重新测量，而不是用库方通用 benchmark 代替项目证据。

### 当前不需要

- schema/codegen、内容协商、多种 wire format；
- 流式 JSON、SSE、WebSocket；
- 为三个请求单独引入 validation 框架。城市业务校验仍由应用 Interface 负责，HTTP 层只做结构与大小校验。

## 4. 配置加载

### 事实

标准库 `flag` 支持字符串、整数、布尔值和 `time.Duration` 等命令行参数；`os.LookupEnv` 能区分“未设置”和“显式设置为空字符串”。[Go 1.23 `flag`](https://pkg.go.dev/flag@go1.23.0)、[Go 1.23 `os.LookupEnv`](https://pkg.go.dev/os@go1.23.0#LookupEnv)

Viper 官方列出的能力包括默认值、flags、环境变量、多种配置文件、远程 KV、动态查找、热更新和 key alias，并定义多来源合并优先级。官方同时不建议使用全局 singleton，因为它让测试更困难、可能出现意外行为；Viper key 不区分大小写，并且同一实例并发读写需要调用方同步。[Viper official README](https://github.com/spf13/viper/blob/master/README.md)

### 对 Job Atlas 的推断

v1 真正需要的进程配置预计只有：

- 监听地址；
- SQLite 路径；
- 启用的 `CompanySource` 及其端点/凭据；
- HTTP server、外部 HTTP client、来源操作和 shutdown 的 timeout；
- worker 并发度（只有出现并发实现时）；
- 日志级别和输出格式。

这些值应在进程启动时一次读取、解析、校验为不可变的强类型设置，再由 composition root 构造 Adapter 和 `Service`。业务模块不应自行读环境变量或 Viper registry。

“已启用来源”直接决定发现任务何时 `completed` 或 `blocked`，现有恢复逻辑也要求未完成工作所需的来源仍被配置。因此运行中热更新来源会改变业务语义；在没有定义旧任务如何冻结配置前，不应把配置热更新当作便利功能加入。

### Interface 与维护成本

- 标准库方案：仓库维护一个小型、纯函数式 loader，把 defaults、env 和 flags 合并成强类型值并集中校验；测试可以直接传入参数/env lookup seam，成本与当前配置项数量成正比。
- Viper：少写一部分多来源读取代码，但需要维护优先级、key 映射、反序列化和 Viper 自身的全局/并发约束。只有确定需要配置文件或多来源合并时，这些成本才有对应收益。

配置包的公开命名要利用包前缀表达语义：例如客户端读作 `config.Load(...)` 与 `config.Settings`，不需要 `config.LoadConfig(...)` 或 `config.ConfigLoader`。Go 官方建议包名成为导出标识符的上下文，避免 `chubby.ChubbyFile` 一类重复。[Go Code Review Comments: Package Names](https://go.dev/wiki/CodeReviewComments#package-names)、[Effective Go: Package names](https://go.dev/doc/effective_go#package-names)

### 当前不需要

- 远程 Consul/etcd、配置加密系统、配置 watcher；
- 多格式配置文件自动发现、alias、运行时 source 切换；
- 通用 `ConfigProvider` Interface。若 loader 只有一个实现，函数输入已经是更小的测试 Seam。

### 候选建议

先确定唯一、可解释的优先级（例如 defaults < env < flags）并使用标准库；只有用户明确需要可编辑配置文件或多来源覆盖时，再以该需求重新评估 Viper 或更窄的解析库。

## 5. 结构化日志

### 事实

Go 1.21 起标准库提供 `log/slog`。它的记录包含时间、级别、消息和键值属性；内置 `TextHandler` 与 `JSONHandler`，并以 `Handler` 作为输出后端 Seam。`Logger.With` 可以预绑定共同属性，`LogAttrs` 可以减少热点分配。[Go official slog introduction](https://go.dev/blog/slog)、[Go 1.23 `log/slog`](https://pkg.go.dev/log/slog@go1.23.0)

Zap 官方定位是高性能结构化分级日志，提供强类型 `Logger` 和更方便但较松的 `SugaredLogger`；官方 benchmark 显示其在若干日志形状中比 `slog` 更快，但 README 也明确提醒 benchmark 有局限，并声明只支持最近两个 Go minor 版本。[Zap official README](https://github.com/uber-go/zap#readme)

### 对 Job Atlas 的推断

当前最重要的是可关联性，而不是极限吞吐。建议的稳定属性包括：

- HTTP 边界：method、route、status、duration；
- 发现任务：`run_id`；
- 工作项：`source_id`、city、attempt/结果；
- 生命周期：启动、恢复工作数、收到信号、停止接入、worker 停止、SQLite 关闭、退出错误。

不要记录完整 JD、可能含 token/query 的申请 URL、来源凭据或整个请求/响应。领域模块应返回带操作上下文的错误；进程、HTTP Adapter 和外部来源 Adapter 在拥有运行事实的边界记录一次，避免每层重复打印同一错误。

### Interface 与维护成本

- 可在 composition root 创建一个 `*slog.Logger`，通过构造参数传给真正需要记录外部运行事实的 Adapter；不应让每个业务类型依赖全局 logger。
- `slog.Handler` 已是替换输出后端的标准 Seam。当前再包一层仓库自定义 `Logger` Interface 会丢失标准属性类型并增加转发代码，除非业务确实需要与日志库完全隔离。
- Zap 需要新增 API、字段类型、flush 与版本维护。只有项目自己的基准或目标日志后端能力显示 `slog` 不足时，才能证明这项成本。

包命名同样应避免重复；若已有 `log` 或 `logging` 包，类型应读作 `logging.Logger` 而不是 `logging.LoggingService`。不过 v1 直接使用 `slog` 时通常连这个包装包都不需要。

### 当前不需要

- 日志采集平台 SDK、trace/metrics 全套 telemetry、运行时热切换 logger；
- 为“以后可能更快”预先引入 Zap；
- 在领域对象上加日志方法或把 logger 放进 `context.Context` 作为隐式依赖。

## 6. HTTP 与来源调用 timeout

### 事实

`http.Server` 提供 `ReadHeaderTimeout`、`ReadTimeout`、`WriteTimeout`、`IdleTimeout` 和 `MaxHeaderBytes`。官方说明多数用户更适合先使用 `ReadHeaderTimeout`，因为 `ReadTimeout` 覆盖整个请求体、无法由 Handler 按请求决定读取策略；零值通常代表没有限制。[Go 1.23 `http.Server`](https://pkg.go.dev/net/http@go1.23.0#Server)

`http.TimeoutHandler` 在 Handler 超时后返回 503，之后 Handler 对响应的写入会得到 `ErrHandlerTimeout`，且不支持 `Hijacker` 或 `Flusher`。[Go 1.23 `http.TimeoutHandler`](https://pkg.go.dev/net/http@go1.23.0#TimeoutHandler)

`http.Client.Timeout` 覆盖连接、重定向和读取响应 body；值为零表示没有 timeout。Client/Transport 应复用而不是每次创建，且可安全并发使用。[Go 1.23 `http.Client`](https://pkg.go.dev/net/http@go1.23.0#Client)、[Go 1.23 `http.Transport`](https://pkg.go.dev/net/http@go1.23.0#Transport)

`context.WithTimeout` 会在期限到达时取消派生 context，调用方仍应调用 cancel 释放关联资源。[Go 1.23 `context.WithTimeout`](https://pkg.go.dev/context@go1.23.0#WithTimeout)

### 对 Job Atlas 的推断

需要分开配置四种不同语义，不能只设置一个全局 timeout：

1. **接入防护**：header timeout、idle timeout、header/body 大小限制；
2. **API 操作 timeout**：一次 SQLite 创建/查询/重启调用的最长等待；
3. **来源操作 timeout**：Nominatim、Overpass、官网/ATS 请求的单次网络边界；
4. **shutdown timeout**：进程允许活跃 Handler 与 worker 收尾的时间。

发现任务本身不能受 HTTP Handler timeout 约束。来源超时先是一次技术执行错误，再由既有重试与 `blocked` 规则决定任务状态。

固定 `WriteTimeout` 需要用“没有分页的完整岗位快照”做最坏情形验证。若对整个 Handler 使用 `TimeoutHandler`，它的默认 503 也可能破坏统一 JSON 错误格式；一个只派生 request context 的小中间件可能更符合现有 Adapter 边界，但仍必须确保 store 调用遵守 context。

### 当前不需要

- 一个 timeout 同时覆盖 HTTP 请求与数小时发现任务；
- 为每次来源请求创建新 `http.Client`；
- 无依据的几十个 timeout 参数。先定义上述四种责任，再用测试/运行数据确定数值。

## 7. 优雅关闭与后台 worker 生命周期

### 事实

`signal.NotifyContext` 可以在指定信号到达时取消 context，返回的 `stop` 应在不再需要信号转发时调用。[Go 1.23 `os/signal.NotifyContext`](https://pkg.go.dev/os/signal@go1.23.0#NotifyContext)

`http.Server.Shutdown(ctx)` 先关闭 listener，再关闭 idle 连接，然后等待活跃连接变 idle；如果 context 过期则返回其错误。调用 Shutdown 后 `ListenAndServe` 返回 `http.ErrServerClosed`，主 goroutine 必须等待 Shutdown 完成后才能退出。Shutdown 不负责 hijacked/WebSocket 连接，而 v1 当前没有这种连接。[Go 1.23 `http.Server.Shutdown`](https://pkg.go.dev/net/http@go1.23.0#Server.Shutdown)

`errgroup` 提供 goroutine 同步、首个错误传播和派生 context 取消；`sync.WaitGroup` 只等待，不表达错误。`errgroup.SetLimit` 还可限制同一 group 的活跃 goroutine 数。[`golang.org/x/sync/errgroup` official docs](https://pkg.go.dev/golang.org/x/sync/errgroup)

Go 官方的 context 指南要求调用链传播取消，并明确指出取消函数本身不会等待工作停止；等待必须由单独的同步机制完成。[Go 1.23 `context`](https://pkg.go.dev/context@go1.23.0)、[Go blog: Contexts and structs](https://go.dev/blog/context-and-structs)

### 对 Job Atlas 的推断

一个可解释的关闭顺序是：

```text
启动：加载并校验配置 → 创建 logger/HTTP clients/sources → Open Service → 启动 HTTP Server

退出信号或致命组件错误
  → 停止接受新 HTTP 请求
  → 在 shutdown deadline 内等待活跃 Handler 返回
  → 取消 worker 生命周期并等待 worker 停止
  → 关闭 SQLite
  → 汇总并记录退出错误
```

先 drain Handler，再关闭它仍会调用的 `Service`/SQLite，可以避免活跃请求访问已关闭资源。不要把信号 context 直接设为所有请求的 `Server.BaseContext` 后又期望 Shutdown 温和 drain：信号取消会先取消这些请求 context。

持久化已经保证异常退出后可以恢复工作，但“可恢复”不等于可以忽略进程内错误。当前 worker 的存储错误会让 worker 静默消失、HTTP 却继续响应。运行时至少需要下列一种显式能力，供架构票最终选择：

- `Service.Run(ctx) error`：调用方启动并等待，worker 致命错误直接返回；
- `Service.Done() <-chan struct{}` + `Service.Err() error`；
- 构造时注入一个只用于上报致命生命周期错误的 callback/channel。

其中前两种更容易让进程将 HTTP server 与 worker 放进同一监督树。若选择 `errgroup.WithContext`，首个组件错误可以取消同组组件并由 `Wait` 返回；若仍只有一个 HTTP server 和一个内部 worker，标准库 channel + WaitGroup 也足够，没必要只为语法简短引入依赖。

当前 `Service.Close()` 没有 context，会等待 worker 无界结束。要使 shutdown deadline 真正生效，还需最终确认：是把生命周期改为可等待的 `Run(ctx) error`，还是要求 `Close` 接受 deadline，或明确所有来源 Adapter 必须及时响应取消并用测试证明。仅在 `main` 外包一层 timeout 不能强行停止一个忽略 context 的 goroutine。

### 当前不需要

- 分布式队列、外部 worker supervisor、cron/scheduler 框架；
- 动态扩缩 worker、leader election、服务发现；
- 为三个固定组件引入依赖注入容器或完整 lifecycle framework。

## 8. 包前缀与导出名称约束

Go 官方指出包名本身就是调用端上下文，所以导出标识符应省略重复包名；例如 `bufio.Reader` 而不是 `bufio.BufReader`。官方也建议避免 `util/common/misc/api/types/interfaces` 等无法约束职责的包名。[Effective Go: Package names](https://go.dev/doc/effective_go#package-names)、[Go blog: Package names](https://go.dev/blog/package-names)

本研究不决定完整目录树，但后续架构票应把下列调用端拼写作为验收：

| 重复拼写 | 利用包前缀后的候选 |
| --- | --- |
| `httpapi.NewHTTPAPIHandler(...)` | `httpapi.NewHandler(...)` |
| `config.LoadConfig(...)` | `config.Load(...)` |
| `config.ConfigLoader` | 一个函数，或确有状态时使用 `config.Loader` |
| `runtime.RuntimeManager` | `runtime.Run(...)` 或职责更具体的包/类型名 |
| `logging.LoggingService` | 直接使用 `*slog.Logger`，或 `logging.Logger` |

这里的右栏只是命名形状，不是最终目录决定。还要避免创建名为 `http` 或 `runtime`、却经常与同名标准库一起导入的本地包；若客户端代码必须频繁重命名 import，通常说明包边界或名称需要调整。

## 9. 需要后续决策票确认的最小问题

1. HTTP 路由是否按当前规模采用 `ServeMux`，并把迁移到 `chi` 的触发条件写入架构基线。
2. JSON 错误 envelope、HTTP 状态映射、请求 body 上限是否成为 HTTP Adapter 的唯一约定。
3. 配置来源与覆盖优先级；服务是否默认只监听 loopback。
4. logger 是否直接采用 `slog`，以及必需/禁止属性清单。
5. 四类 timeout 的配置字段与验收方法，尤其是无分页 `GetDiscovery` 的响应上界。
6. worker 致命错误如何上报给进程，以及 shutdown deadline 怎样穿过 `Service` 和各来源 Adapter。

这些问题完成后，运行时基础栈才足以支撑目录与依赖方向设计；当前没有事实要求引入 Web 框架、Viper、Zap、分布式队列或 lifecycle 容器。
