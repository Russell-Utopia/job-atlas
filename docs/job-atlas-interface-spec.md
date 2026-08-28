# Job Atlas Interface 规格

## 1. 目标与范围

Job Atlas 接收城市列表，异步发现当前仍可读取、具有完整 JD 和岗位级投递入口的具体岗位。第一版提供 API 交互模块，不提供 Web 交互模块，也不执行投递。

“全部结果”只表示本次已启用来源完成扫描后得到的全部合格岗位，不表示现实世界中的绝对全集。

## 2. 对外 Interface

```text
StartDiscovery(cities) -> runId

GetDiscovery(runId) -> {
  status,
  jobs,
  error
}

RestartDiscovery(runId)
```

`status` 只有：

- `running`：仍在扫描或自动重试。
- `completed`：所有已启用来源及其产生的下游工作均已结束。
- `blocked`：自动重试结束后，仍有已启用范围无法完成。

`jobs` 每次返回当前已经确认的完整岗位快照；任务仍为 `running` 时也可以出现新岗位。每个岗位固定为：

```text
Job {
  companyName
  title
  city
  jd
  applyUrl
}
```

`error` 是可空的通用任务错误，只说明当前阻止任务完成的原因，不预设错误来自哪类来源。候选级排除或已经解决的历史错误不进入这里。

第一版不提供取消、分页、游标或对外过期机制。

## 3. 岗位输出门槛

岗位只有同时满足以下条件才进入 `jobs`：

1. 具体岗位页面在本轮检查时仍可读取。
2. 页面展示明确的招聘发布主体、岗位名称与目标城市。
3. `jd` 是该具体岗位的完整职责和要求，不是招聘简介。
4. `applyUrl` 属于该具体岗位并能进入申请流程。
5. 岗位来自可信招聘路径：可以位于企业自有域名，也可以位于经企业官方路径确认的企业专属 ATS。

`companyName` 取具体岗位页面展示的招聘发布主体，不推断劳动合同主体，也不要求证明当地公司与招聘发布主体的法律从属关系。

## 4. 三个业务深模块

### 4.1 RegionalCompanyDiscovery

```text
DiscoverCompanies(CityScope) -> CompanyInfo[]
```

输入调用方声明的城市范围，增量输出已启用企业来源在该城市中发现的企业信息。模块内部隐藏来源查询方式、城市边界处理、企业名称补全、同一企业合并和证据保存；不要求先发现产业园。

`CompanyInfo` 只包含下一模块寻找招聘来源所需的最小事实：来源中的企业名称、城市、证据引用，以及来源能够提供的别名、位置或登记标识。名称、城市和证据引用是必需事实；其余字段有则保存，不要求补成完整工商档案。

内部 Seam：

- `CompanySource`：按一个城市扫描自身声明的来源范围并增量产生企业候选。第一版的 OSM Adapter 在内部解析并缓存城市行政边界，直接扫描边界内命中声明规则的 OSM 对象，不经过产业园步骤。
- `CompanyLookupSource`：补全已知企业；第一版保留 Seam，但不启用企查查、天眼查或政府企业数据 Adapter。

OSM 来源完成只表示城市边界内、当前 OSM 数据快照中命中本次规则的对象已经扫描完成，不表示城市工商企业全量。

### 4.2 OfficialJobDiscovery

```text
DiscoverJobPostings(CompanyInfo) -> JobPostingSnapshot[]
```

输入单个企业信息，增量输出该企业的具体岗位页面快照。模块内部隐藏官网确认、招聘入口发现、列表分页、自建招聘页及 Moka、Hotjob 等 ATS 差异。

招聘首页和岗位列表只是内部导航，不是模块输出。每个 `JobPostingSnapshot` 对应一个具体岗位，保存当前企业引用、来源岗位 ID、具体岗位 URL、页面原始公司名/岗位名/城市/JD、岗位级投递入口、检查时间和证据引用。

模块内部通过 `RecruitmentParser` 策略 Seam 隔离自建招聘页、Hotjob、Moka 等解析差异：

- `OfficialJobDiscovery` 内部的调用方先确认并分类招聘路径，再根据来源类型选择解析策略；`DiscoveryPlanner` 不参与策略选择。
- 解析策略只接收该来源解析所需的 URL、租户标识、岗位标识或分页信息，不接收完整 `CompanyInfo`。
- 解析策略返回来源页面中的岗位事实；调用方再与企业引用和招聘路径证据组合成 `JobPostingSnapshot`。
- 解析策略不决定 `companyName`，也不判断岗位是否进入对外结果。

### 4.3 JobResultBuilder

```text
BuildJob(JobPostingSnapshot) -> JobDecision
```

输入一个具体岗位页面快照，完成字段清洗、`companyName` 取值、城市判断、完整 JD 与岗位级投递入口校验以及岗位去重。

`JobDecision` 只有：

- `included`：携带最终 `Job`。
- `excluded`：已得到明确业务结论，但不满足岗位输出门槛。
- `error`：技术执行没有得到业务结论。

每个结论携带具体 `reason`；只有 `error` 需要用 `retryable` 表示是否自动重试。当前不预先穷举封闭错误码。

## 5. 编排与并行

`DiscoveryPlanner` 负责把三个模块连接起来，但不理解网页、供应商或来源字段：

1. 每个城市形成 `CityScope` 并直接进入 `RegionalCompanyDiscovery`；具体来源如何解析城市范围由其 Adapter 负责。
2. 每发现一个 `CompanyInfo`，立即投递给 `OfficialJobDiscovery`，不等待该城市所有企业来源扫描结束。
3. 每发现一个 `JobPostingSnapshot`，立即投递给 `JobResultBuilder`，不等待该企业其他来源扫描结束。
4. 每产生一个 `included`，立即写入当前岗位结果，供后续 `GetDiscovery` 返回。

不同城市、企业和岗位可以并行；同一条证据链仍按顺序推进。三个逻辑工作流通道可以共用一套持久化队列实现。

## 6. 完成、错误与重启

- 模块按已配置范围是否扫描到终点判断完成，不按是否找到结果判断。
- 完整扫描后没有企业、招聘路径或目标城市岗位，可以正常完成。
- 网络超时、限流、临时服务错误和可继续的分页中断可以自动重试。
- 明显无法靠原样重试解决的问题不自动重试；若其阻止已启用范围完成，任务进入 `blocked`。
- `RestartDiscovery` 继续原任务，重新激活所有未完成工作项，不受旧 `retryable` 限制；已完成工作不重做。
- 每次失败与重试追加执行记录，但不保存旧页面、旧 URL、旧 JD 或旧内容指纹。后续成功成为当前事实，顶层 `error` 不再保留已解决错误。

## 7. 来源启用规则

- 只有具备访问权限并明确配置进本次任务的来源才是已启用来源。
- 未启用来源不影响 `completed`。
- 已启用来源扫描失败且自动重试耗尽后，任务为 `blocked`，不能静默当作零结果。
- 第一版不启用企查查、天眼查、国家企业信用信息公示系统或地方政府企业数据。
- 后续取得企业账号、商务或政府授权、额度和明确数据使用许可后，可以在 `CompanySource` 或 `CompanyLookupSource` Seam 增加 Adapter，不改变业务模块与对外 Interface。

## 8. 岗位身份与更新

- 相同来源岗位 ID 表示同一岗位。
- 企业官网与企业专属 ATS 之间存在可验证的同一岗位跳转关系时，对外只返回一条岗位。
- 仅公司、城市、标题或 JD 相似不足以合并岗位。
- 来源岗位 ID 不变而标题、JD 或 URL 改变时，更新同一岗位的当前事实。
- 来源岗位 ID 改变时默认形成新岗位，除非来源明确提供跳转或替换关系。
- 系统只保存当前岗位事实、当前来源岗位 ID、当前 URL 和最近检查时间，不保留旧岗位内容。

## 9. 验收场景

1. 一个城市仍在扫描时，已满足门槛的岗位已经出现在 `GetDiscovery.jobs`。
2. 已启用来源全部扫描完成且结果为空时，任务为 `completed`、`jobs=[]`。
3. 一个岗位缺少完整 JD 或岗位级投递入口时，它不进入 `jobs`，其他岗位不受影响。
4. 一个已启用来源无法完成且自动重试耗尽时，任务为 `blocked`，`error` 说明当前原因；已确认岗位仍可读取。
5. 修复网络、配置或 Adapter 后调用 `RestartDiscovery`，原任务从未完成工作继续，已完成岗位不重做。
6. 企业官网链接到企业专属 ATS 的具体岗位时，该 ATS 岗位可以成为可信岗位结果。
7. 同一岗位同时出现在官网与 ATS 时，对外只返回一条；同名但无同一岗位证据时分别保留。
8. 只提交城市、不提供产业园时，OSM Adapter 可以直接扫描城市行政边界并产生企业候选；完成只表示当前 OSM 规则范围扫描结束。

## 10. 证据依据

- [可行性与领域设计地图](../.scratch/job-atlas/map.md)
- [企业候选到官方网站研究](../.scratch/job-atlas/research/company-candidate-to-official-domain.md)
- [商业与政府企业数据可访问性研究](../.scratch/job-atlas/research/company-data-source-accessibility.md)
- [从城市范围直接发现企业候选研究](../.scratch/job-atlas/research/city-direct-company-discovery.md)
- [福州/长沙现有审计原型](/Users/chenjunzhou/Documents/Codex/2026-08-27/x20/work/fuzhou-changsha-job-proof)
