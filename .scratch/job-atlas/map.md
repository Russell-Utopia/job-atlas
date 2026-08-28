# Job Atlas 可行性与领域设计地图

Label: wayfinder:map

## Destination

形成一份可交给 `/to-spec` 的 Job Atlas 可行性与领域设计：对外只暴露稳定的岗位发现 Interface，内部能说明从城市到可投岗位的证据链、失败边界和重跑语义。

## Notes

- 每次继续地图时使用 `domain-modeling`；涉及外部 Interface 或内部 Seam 时同时使用 `codebase-design`。
- 当前只做决策，不写业务代码。
- “所有岗位”只指本轮从已配置数据源完成扫描并满足输出门槛的岗位，不承诺现实世界的绝对全集。
- 直接复用已有证据与审计原型，不重新采集或制作：[福州/长沙审计原型](/Users/chenjunzhou/Documents/Codex/2026-08-27/x20/work/fuzhou-changsha-job-proof)、[audit.json](/Users/chenjunzhou/Documents/Codex/2026-08-27/x20/work/fuzhou-changsha-job-proof/public/data/audit.json)、[官方证据清单](/Users/chenjunzhou/Documents/Codex/2026-08-27/x20/work/fuzhou-changsha-job-proof/public/data/official-evidence-manifest.json)、[原始资料压缩包](/Users/chenjunzhou/Documents/Codex/2026-08-27/x20/outputs/%E5%9B%AD%E5%8C%BA%E5%8F%91%E7%8E%B0%E5%AE%A1%E8%AE%A1%E6%95%B0%E6%8D%AE-%E7%A6%8F%E5%B7%9E%E9%95%BF%E6%B2%99-2026-08-27.zip)。

## Decisions so far

- [从城市范围直接发现企业候选](research/city-direct-company-discovery.md)：第一版不再经过“城市→产业园→园区周边企业”，而由 OSM Adapter 直接扫描城市行政边界内命中声明规则的企业候选；`RegionalCompanyDiscovery` 改为接收 `CityScope`，内部使用中性的 `CompanySource` Seam。OSM 扫描完成不表示城市工商企业全量。
- 招聘页面解析采用 `RecruitmentParser` 策略 Seam：`OfficialJobDiscovery` 内部调用方根据已经确认的来源类型选择自建页面、Hotjob 或 Moka 策略；策略不接收完整 `CompanyInfo`，也不负责最终岗位判断。
- [确定第一版企业数据来源启用策略](https://github.com/Russell-Utopia/job-atlas/issues/10)：第一版不启用企查查、天眼查或政府企业数据；只保留内部 `CompanyLookupSource` Seam，未来取得账号、授权和额度后再按配置启用。
- [验证商业与政府企业数据来源的可访问性](https://github.com/Russell-Utopia/job-atlas/issues/9)：企查查、天眼查可在企业认证、审核和付费后补全已知企业，但不能证明区域企业查全；国家公示系统没有确认面向普通开发者的全国公开 API，地方政府数据需逐城市验证。
- [定义岗位身份与跨来源去重](https://github.com/Russell-Utopia/job-atlas/issues/8)：相同来源岗位 ID 或可验证的官网到企业专属 ATS 跳转关系代表同一岗位；来源岗位 ID 不变时直接更新当前岗位事实，不保存旧页面、旧 JD、旧 URL 或旧内容指纹。标题、城市和 JD 相似不能单独触发合并。
- [定义最小失败与重试语义](https://github.com/Russell-Utopia/job-atlas/issues/5)：候选只使用 `included / excluded / error`，`blocked` 只属于整个发现任务；具体 `reason` 可扩展而不预先穷举，`retryable` 只控制自动重试，Restart 会重新激活全部未完成工作并保留不含旧 JD 的执行尝试记录。
- [确定三个业务深模块与内部数据源 Seam](https://github.com/Russell-Utopia/job-atlas/issues/4)：规划模块只编排“城市范围到企业信息、企业信息到具体岗位页面快照、岗位页面快照到最终岗位”三个深模块；早期 `MapArea` 输入已由后续城市直查研究收敛为 `CityScope`。企查查、政府数据、官网确认、列表扫描和 ATS 差异都隐藏在模块内部。
- [验证企业候选到官方网站的自动发现](https://github.com/Russell-Utopia/job-atlas/issues/7)：冷启动样本证明该步骤条件可行；搜索只负责召回，须由企业自有页面与另一项一手锚点交叉确认，并保留候选域名和拒绝理由。
- [定义发现任务的控制与结果读取策略](https://github.com/Russell-Utopia/job-atlas/issues/6)：第一版只提供创建、查询、重启，查询返回已确认岗位的完整快照；取消、分页、游标和对外过期暂不加入。
- [定义“当前可投岗位”的证据门槛](https://github.com/Russell-Utopia/job-atlas/issues/3)：以本轮岗位级申请流程有效为准；候选使用 `included / excluded / error` 并强制记录具体原因，`blocked` 只用于发现任务。
- 岗位返回只以目标城市、具体岗位仍存在、完整 JD、岗位级投递入口和可信来源为硬门槛；园区归属及公司法律关系不阻塞返回，只保留最小内部来源轨迹。
- [定义岗位 companyName 的主体语义](https://github.com/Russell-Utopia/job-atlas/issues/2)：`companyName` 取岗位页招聘发布主体；法律从属不是输出门槛，系统验证的是可审计的官方招聘路径。
- [定义发现任务的完成语义](https://github.com/Russell-Utopia/job-atlas/issues/1)：岗位结果与任务控制分离；任务可查询、自动重试并支持原任务重启，阻塞原因由通用 `error` 表达。
- 调用方只接收 `Job { companyName, title, city, jd, applyUrl }`；园区、企业来源和抓取过程属于内部实现。
- `jd` 必须是具体岗位的完整 JD；`applyUrl` 必须属于该岗位并能进入投递流程。缺任一项的候选不进入对外结果，但保留内部记录。
- 三个业务深模块内部按实际发生的来源与处理阶段保留追加式发现记录，并区分 `included`、`excluded`、`error`；每条结果都记录明确 `reason` 与 `retryable`。

## Not yet specified

- 当前决策地图没有未解决票据；已收敛为 [ADR-0001](../../docs/adr/0001-asynchronous-three-module-discovery.md) 与 [Interface 规格](../../docs/job-atlas-interface-spec.md)。

## Out of scope

- 本地图不实现业务代码，也不建设全国企业或岗位目录。
- 不承诺发现现实世界中的绝对全部岗位。
- 不代替用户执行岗位投递。
