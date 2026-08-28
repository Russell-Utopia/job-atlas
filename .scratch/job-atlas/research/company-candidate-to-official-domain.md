# 企业候选到官方网站：冷启动可行性验证

核验时间：2026-08-28。输入只取既有 [`audit.json`](/Users/chenjunzhou/Documents/Codex/2026-08-27/x20/work/fuzhou-changsha-job-proof/public/data/audit.json) 中的候选名称、城市和园区线索；没有用交接文档中的官网 URL 起步，也没有改写原始数据或原型。

## 结论

这一步可以自动化，但不能实现为“搜索企业名，取第一个域名”。三条 `audit.json` 候选均从冷启动输入找到了可信的当前企业域名：

| 候选 | 结果 | 选中域名 | 难度 |
|---|---|---|---|
| 长沙广立微电子有限公司 | `included` | `semitronix.com` | 直接命中；官网、地址和深交所资料相互吻合 |
| 金码测控科技有限公司 | `included` | `kingmach.cn` | 候选名是旧/简称，靠地址和证券代码确认当前全称 |
| 高意科技 | `included` | `coherent.com` | 品牌名不完整且搜索首屏出现过时的 `corning.com` 页面，必须解析集团沿革和当前福州主体 |

另以“福建顶点软件股份有限公司”作补充对照，得到 `apexsoft.com.cn`；但它**不在本次 `audit.json` 的候选列表中**，不能写成第三条本地候选。

因此，样本证明“候选名 + 城市 → 官方域名”可走通，也暴露了同名、简称、母子公司和过时一手页面的风险；四个样本不足以证明任意企业都能自动确认，更没有证明后续岗位发现链路已自动走通。

## 最小证据门槛

候选域名只有同时满足以下条件才可 `included`：

1. 从“候选原名 + 城市/园区 + 官网”开始召回，保存查询、候选域名和排序；商业目录、搜索摘要只负责召回。
2. 域名内的一手页面明确出现候选企业、可解释的集团品牌/现主体，且城市、地址或业务至少有一项与输入吻合。
3. 再有一个防仿冒锚点：交易所/挂牌公司披露写明官网，政府材料连接到同一主体，或当前集团官网的主体/地址清单确认该实体。仅有招聘平台、商业目录或同名网页不够。
4. 企业名不一致时，不凭字面相似合并；必须保留“候选名 → 当前主体/集团 → 官方域名”的一手来源路径。Job Atlas 不需要证明劳动合同主体，但必须能解释为什么这个域名属于该候选线索。
5. 遇到主体变更或互相冲突的一手页面，比较页面时效、当前地址和最新官方披露；旧官网页面不能覆盖更新的一手材料。

## 冷启动记录

### 长沙广立微电子有限公司

输入来自 `audit.json`：距中电软件园二期约 110 米，来源 `way/1296828284`。

主要候选域名：

- **接受 `semitronix.com`**：官网[联系页](https://www.semitronix.com/contact/)自称“杭州广立微电子股份有限公司”，当前列出长沙分部“尖山路18号中电软件园二期E6栋”；[深交所公司要览](https://static.cninfo.com.cn/finalpage/enpage/301095_1.pdf)把 `www.semitronix.com` 列为股票代码 301095 的官方网站；官网[长沙子公司文章](https://www.semitronix.com/news/company-info/696.html)又明确出现“长沙广立微电子有限公司”。名称、城市、地址和上市主体闭环。
- **不把 `cninfo.com.cn`、`szse.cn` 当公司官网**：它们是监管/披露证据载体，不是企业自有域名。
- **拒绝 `zhaopin.com`、`mokahr.com` 作为官网**：前者是招聘平台，后者即使由官网链接到企业专属招聘站，也只是招聘承载域名；可作为后续岗位来源，不能覆盖公司官网语义。

### 金码测控科技有限公司

输入来自 `audit.json`：距长沙软件园约 813 米，来源 `node/7066624810`，地图名缺少“长沙”和“股份”字样。

主要候选域名：

- **接受 `kingmach.cn`**：公司[首页及联系信息](https://www.kingmach.cn/)明确显示“长沙金码测控科技股份有限公司”、证券代码 872288、桐梓坡西路188号；地址与候选位置吻合。公司 2025 年年度报告也写明同一公司名、地址和网址，报告可从[全国股转系统挂牌公司查询](https://www.neeq.com.cn/nq/listedcompany.html)按 872288 获取。
- **拒绝 `kingmachmeasure.com`**：它只在 LinkedIn 企业档案中出现，没有从当前公司页面、挂牌披露或政府材料获得一手确认。
- **拒绝 `everiaction.com` 作为官网**：它是 `kingmach.cn` 页面链接的产品云平台，不是金码测控的公司主页。
- 招聘站和商业数据库只用于召回；即使其名称/地址吻合，也不参与最终认定。

### 高意科技

输入来自 `audit.json`：距福州软件园（晋安分园）约 409 米，来源 `way/1230004289`；它只是品牌/地点名，不是完整法人名。

主要候选域名：

- **接受 `coherent.com`**：Coherent 当前[主体清单](https://www.coherent.com/legal/list-of-the-subsidiaries)列出位于福州软件园晋安分园的 `Coherent China, Inc.`，以及福新东路253号的 `Fuzhou Photop Optics Co., Ltd.`；其[福州地点页](https://www.coherent.com/company/locations/apac/china/coherent-fuzhou-photonics)亦列出福州 Photop 主体。晋安区政府早期材料把该园区实体称为“[II-VI高意亚太总部](https://www.fzja.gov.cn/xjwz/zwgk/zfxxgkzdgz/zdjsxm/jsqk/201811/t20181115_2677501.htm)”，市场监管总局则记录了“[高意收购相干](https://www.samr.gov.cn/jzxts/tzgg/ftjpz/art/2022/art_bc228f606ab44f299974c43e3d6069fd.html)”的集团变化。该路径足以把地图品牌线索连接到当前集团域名。
- **拒绝 `corning.com` 作为当前官网**：其[福州旧招聘页](https://www.corning.com/careers/cn/zh/careers/Location/Fuzhou.html)确实写着“福州高意科技有限公司”和旧址，但页脚停在 2020 年；它与 Coherent 当前主体清单、当前福州地点和集团沿革冲突。此例证明“一手网站”也可能过时，不能只看域名权威性。
- **拒绝 `zhipin.com`、`linkedin.com`、`jobcn.com`**：均为招聘/职业平台，且展示的公司名、地址并不完全一致，只能产生后续核验线索。

### 福建顶点软件股份有限公司（补充对照，非 audit 候选）

- **接受 `apexsoft.com.cn`**：公司 2025 年年度报告明确写明公司全称、福州软件园地址和 `www.apexsoft.com.cn`，可从[上交所股票与存托凭证查询](https://www.sse.com.cn/assortment/stock/home/)按 603383 获取；[发行人年度报告文本](https://pdf.dfcfw.com/pdf/H2_AN202604151821244037_1.pdf)也保留了相同字段。
- **拒绝 `apexinfo.com.cn`**：搜索命中的是“顶点政企”的校园招聘信息，不能据此替代“福建顶点软件股份有限公司”的官网。
- **拒绝 `tiptop.cn`**：页面主体是厦门顶点软件有限公司，城市和法人名均不符，属于同名风险。
- **拒绝 `zhaopin.com` 等招聘站**：只能证明平台上存在相应公司/岗位页面，不能证明公司官网。

## 结果与任务状态

仍沿用现有候选结果模型：

| 情形 | 结果 | 示例 reason | retryable |
|---|---|---|---|
| 官方域名达到上述门槛 | `included` | `OFFICIAL_DOMAIN_CONFIRMED` | `false` |
| 同名但城市/主体不符 | `excluded` | `UNRELATED_SAME_NAME` | `false` |
| 只有招聘平台、目录或无法确认的域名 | `excluded` | `OFFICIAL_DOMAIN_NOT_CONFIRMED` | `true` |
| 页面明确过时且有更新一手材料否定 | `excluded` | `STALE_OR_CONFLICTING_SITE` | `false` |
| 搜索超时、限流、403 或解析失败 | `error` | `SEARCH_TIMEOUT` / `SOURCE_UNAVAILABLE` | `true` |

`blocked` 不用于单个候选。技术错误应先自动重试；重试耗尽且导致本轮配置的数据源/查询范围没有扫描完整时，才把**整个发现任务**标为 `blocked`，在任务顶层 `error` 中说明未完成范围。某个候选经过完整核验后得到 `excluded`，不应阻塞整个任务。

## 查询日志

以下是本次实际使用的全部搜索字符串。带域名的查询只在无域名冷启动搜索已召回该域名后用于复核，没有反向使用交接 URL。

<details>
<summary>长沙广立微（7 条）</summary>

1. `长沙广立微电子有限公司 长沙 官网`
2. `"长沙广立微电子有限公司" 官网`
3. `"长沙广立微电子有限公司" 招聘`
4. `杭州广立微电子股份有限公司 2025 年年度报告 公司网址 semitronix.com`
5. `site:cninfo.com.cn 301095 公司网址 semitronix.com`
6. `site:szse.cn 301095 semitronix.com`
7. `site:semitronix.com 联系我们 长沙 中电软件园`

</details>

<details>
<summary>金码测控（11 条）</summary>

1. `金码测控科技有限公司 长沙 官网`
2. `"长沙金码测控科技股份有限公司" 网址 872288`
3. `"金码测控科技有限公司" 招聘`
4. `site:neeq.com.cn 872288 kingmach.cn`
5. `site:neeq.com.cn 金码测控 公司网址`
6. `"长沙金码测控科技股份有限公司" "www.kingmach.cn"`
7. `"金码测控科技有限公司" "桐梓坡西路188号"`
8. `site:neeq.com.cn/disclosure/2026 "金码测控"`
9. `site:neeq.com.cn "872288" "2025年年度报告"`
10. `site:neeq.com.cn "长沙金码测控科技股份有限公司"`
11. `全国中小企业股份转让系统 金码测控 872288 官网`

</details>

<details>
<summary>高意科技（16 条）</summary>

1. `高意科技 福州 官网`
2. `"高意科技" 福州`
3. `"福州高意科技有限公司" 官网`
4. `"福州高意科技有限公司" site:coherent.com`
5. `"福州高意科技有限公司" site:corning.com`
6. `"Fuzhou Photop Technologies" official`
7. `福州高意科技 Corning 收购 高意 官方`
8. `site:corning.com "Photop Technologies" acquisition`
9. `site:coherent.com "Fuzhou" careers China`
10. `site:sec.gov "Fuzhou Photop Technologies" 2025`
11. `Corning acquired Photop Technologies Fuzhou 2025 2026`
12. `site:corning.com/careers "Photop Technologies, Inc."`
13. `site:coherent.com/careers Fuzhou Photop`
14. `"Photop Technologies, Inc." Corning Coherent`
15. `"高意科技" 福州 招聘`
16. `"福州高意科技有限公司" "官网"`

</details>

<details>
<summary>福州顶点补充对照（10 条）</summary>

1. `福州 顶点 软件 官网`
2. `"福建顶点软件股份有限公司" 官网`
3. `"福建顶点软件股份有限公司" "apexsoft.com.cn"`
4. `site:sse.com.cn "福建顶点软件股份有限公司" "公司网址"`
5. `site:apexsoft.com.cn "福建顶点软件股份有限公司"`
6. `"福建顶点软件股份有限公司" 招聘`
7. `site:sse.com.cn/assortment/stock/list/info/company 603383`
8. `site:sse.com.cn 603383 顶点软件 公司资料`
9. `上海证券交易所 603383 顶点软件 公司网址`
10. `site:static.sse.com.cn 603383 2025 年年度报告`

</details>

## 对后续设计的约束

- 保存 `query`、搜索时间、主要候选域名、接受/拒绝理由和证据 URL；否则无法审计“为什么选了这个官网”。
- 官方域名发现是可单独重试的步骤；搜索或页面读取错误不能伪装成“没有官网”。
- 集团/子公司关系只需支撑可信发现路径，不扩张成劳动合同主体建模。
- 招聘平台域名可以在已确认官网的链接或其他一手材料背书后，作为后续招聘来源；它本身不等于公司官网。
