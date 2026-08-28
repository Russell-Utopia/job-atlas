# 从城市范围直接发现企业候选的可行性核验

核验时间：2026-08-28。

本报告只读取既有福州、长沙审计数据，并查阅 OSM、Overpass 和 Nominatim 的一手文档；没有重新请求、采集或制作两地原始数据。

## 结论

**可以跳过“城市 → 产业园/载体 → 园区周边企业”这层绕路，直接从城市范围查询企业候选。**

原因是原型的“载体发现”和“企业发现”本来就使用同一个数据源：OpenStreetMap 数据及 Overpass API。产业园不是另一份企业数据库，只是原型为了缩小空间范围采用的召回启发式。Overpass 既支持 `around` 半径过滤，也支持城市 `area` 和 `bbox` 空间过滤；因此可以把“载体中心 900 米内”替换成“城市行政区内”。[Overpass `area` 过滤器](https://wiki.openstreetmap.org/wiki/OverpassQL#By_area_(area))会选择给定区域内的节点、道路和关系；[`bbox` 过滤器](https://wiki.openstreetmap.org/wiki/Overpass_API/Language_Guide#Bounding_box_clauses_(%22bbox_query%22,_%22bounding_box_filter%22))也可以按南、西、北、东坐标限制对象。

但必须使用准确口径：

- 能得到：**城市边界内已经被 OSM 贡献者标注，并且命中企业名称或企业相关标签规则的 OSM 对象**。
- 不能得到：**该城市全部工商登记企业**，也不能据此证明企业仍存续、正在招聘或其名称就是法定主体名称。OSM 官方 Wiki 明确说明地图永远不会完整，且不同地区、不同对象类型的覆盖程度差异很大。[OSM Completeness](https://wiki.openstreetmap.org/wiki/Completeness)

因此，直接城市查询适合作为 Job Atlas 第一版的一个真实 `CompanySource`，前提是把它命名和验收为 **OSM 企业候选来源**，不把它描述成企业工商库或城市企业全量来源。产业园链路可以删除出必经主流程，日后若实测能提高召回率，再作为同一来源内部的可选补充策略。

## 1. 原型实际做了什么

### 1.1 城市边界

原型保存的边界数据来自 OpenStreetMap Nominatim：

- 福州命中 OSM `relation/3263977`，返回 `MultiPolygon` 和 bbox `25.0958835,118.3759615,26.6384422,120.7220227`：[fuzhou-00-boundary.raw.json](/Users/chenjunzhou/Documents/Codex/2026-08-27/x20/work/fuzhou-changsha-job-proof/public/data/fuzhou-00-boundary.raw.json)
- 长沙命中 OSM `relation/3202711`，返回 `Polygon` 和 bbox `27.8512095,111.8908381,28.6644154,114.2560358`：[changsha-00-boundary.raw.json](/Users/chenjunzhou/Documents/Codex/2026-08-27/x20/work/fuzhou-changsha-job-proof/public/data/changsha-00-boundary.raw.json)

Nominatim 的正式 Search API 支持 `polygon_geojson=1` 返回地点完整几何；这与两个 raw 文件包含完整 `geojson` 的形态一致。[Nominatim Search API](https://nominatim.org/release-docs/latest/api/Search/#polygon-output)

需要明确一个审计缺口：**仓库只保存了 Nominatim 响应，没有保存当时的请求 URL、查询文本和完整参数。** 因此可以确认边界对象、bbox、完整几何和数据来源，不能声称已经还原出当时精确的 Nominatim 请求字符串。

### 1.2 载体发现

载体阶段在两个城市的 bbox 内执行以下 Overpass 查询模板：

```text
nwr["name"~"软件园|科技园|产业园|工业园|创业园|孵化器|众创空间|商务楼|商务中心|写字楼|大厦"](south,west,north,east);
out center tags;
```

该模板由审计 manifest 和现有页面共同保存：[audit.json](/Users/chenjunzhou/Documents/Codex/2026-08-27/x20/work/fuzhou-changsha-job-proof/public/data/audit.json)、[page.tsx](/Users/chenjunzhou/Documents/Codex/2026-08-27/x20/work/fuzhou-changsha-job-proof/app/page.tsx)。

原始响应分别含福州 235 个、长沙 327 个 OSM 对象，随后才用 Nominatim 的 Polygon/MultiPolygon 做点在面内裁切。因此原型的载体发现本身已经是“城市 bbox 查询 → 本地行政边界裁切”，并不是先从政府产业园数据库取得园区：[fuzhou-01-carriers.raw.json](/Users/chenjunzhou/Documents/Codex/2026-08-27/x20/work/fuzhou-changsha-job-proof/public/data/fuzhou-01-carriers.raw.json)、[changsha-01-carriers.raw.json](/Users/chenjunzhou/Documents/Codex/2026-08-27/x20/work/fuzhou-changsha-job-proof/public/data/changsha-01-carriers.raw.json)。

### 1.3 企业发现

企业阶段没有查询政府或工商企业数据库。它从载体排名中选择 `software_park`、`technology_park`、`incubator` 三类的前四名，然后对每个载体中心执行 900 米半径查询。

福州的精确请求是：

```text
[out:json][timeout:120];
(
nwr(around:900,26.0897456,119.3524949)["name"];
nwr(around:900,26.1226306,119.272571)["name"];
nwr(around:900,26.1031053,119.2413109)["name"];
nwr(around:900,26.1027821,119.2442547)["name"];
);
out center tags;
```

长沙使用相同模板和四个长沙载体中心。两个完整请求及选择规则已经保存在：[fuzhou-05-enterprise-search.request.json](/Users/chenjunzhou/Documents/Codex/2026-08-27/x20/work/fuzhou-changsha-job-proof/public/data/fuzhou-05-enterprise-search.request.json)、[changsha-05-enterprise-search.request.json](/Users/chenjunzhou/Documents/Codex/2026-08-27/x20/work/fuzhou-changsha-job-proof/public/data/changsha-05-enterprise-search.request.json)。Overpass 官方文档也确认 `around:radius,latitude,longitude` 的含义就是选择给定坐标一定米数内的对象。[Overpass `around` 过滤器](https://wiki.openstreetmap.org/wiki/OverpassQL#Relative_to_other_elements_(around))

关键点是，这个请求只要求对象具有 `name`；它返回的不是“企业表”，而是附近所有带名称的 OSM 节点、道路和关系。原始响应包含福州 270 个、长沙 555 个对象，之后审计规则才排除道路、公共设施、住宅、载体本身等对象：[fuzhou-06-enterprises.raw.json](/Users/chenjunzhou/Documents/Codex/2026-08-27/x20/work/fuzhou-changsha-job-proof/public/data/fuzhou-06-enterprises.raw.json)、[changsha-06-enterprises.raw.json](/Users/chenjunzhou/Documents/Codex/2026-08-27/x20/work/fuzhou-changsha-job-proof/public/data/changsha-06-enterprises.raw.json)。

这证明产业园在原型中只有两个作用：缩小查询范围、给候选增加“离某载体多近”的评分；它并不提供企业身份事实。原型自己也明确把空间接近解释为候选线索，不能证明企业属于园区。

## 2. 如何直接按城市查询

### 2.1 空间范围有两种可行方式

1. **Overpass area**：用已经确认的行政区 relation 映射为 area，再查询 area 内对象。Overpass 官方文档说明 area 是服务端根据 OSM relation/closed way 派生的数据，area 数据通常比主库延迟数小时，而且并非每个 relation 都一定存在对应 area。[Overpass Areas](https://wiki.openstreetmap.org/wiki/Overpass_API/Areas)、[Overpass `area` 过滤器](https://wiki.openstreetmap.org/wiki/OverpassQL#By_area_(area))
2. **bbox 查询 + 本地 polygon 裁切**：沿用原型已有办法，先在 bbox 内查询，再用保存的行政区 GeoJSON 排除 bbox 内但行政区外的对象。它不会依赖 Overpass area 的生成延迟，也便于把大城市拆成可恢复的 bbox 分片。

Job Atlas 第一版更适合第二种方式，因为它复用已验证的数据形态，且能把“扫描到终点”定义为：所有城市 bbox 分片和所有企业候选过滤分支均成功完成，随后行政区裁切完成。这里不需要先查产业园。

### 2.2 不能把原来的 `[name]` 查询原样放大到整座城市

对整座城市执行 `nwr["name"]` 会召回道路、社区、商店、学校、景点和大量其他有名对象，既昂贵又会产生极大噪声。城市级查询应在服务端先限制为若干企业候选分支，例如：

```text
(
  nwr["name"]["office"](city_bbox);
  nwr["name"]["craft"](city_bbox);
  nwr["name"]["man_made"="works"](city_bbox);
  nwr["name"]["industrial"](city_bbox);
  nwr["name"~"有限公司|股份有限公司|集团|研究院|运营中心|工厂"](city_bbox);
);
out center tags;
```

这只是建议的查询形状，并未在本次核验中运行。实际规则应版本化，并继续执行行政区裁切、非企业排除、同一 OSM 对象去重和同名候选合并。

不能只依赖 `office/craft/works/industrial` 标签。既有审计结果中，福州 11 个已接受候选只有 7 个带这组企业相关标签；长沙 113 个已接受候选只有 10 个带这组标签，大量公司是“名称带公司主体词的建筑对象”。这些数字来自既有审计文件，不是本次重新采集：[fuzhou-07-enterprises.audited.json](/Users/chenjunzhou/Documents/Codex/2026-08-27/x20/work/fuzhou-changsha-job-proof/public/data/fuzhou-07-enterprises.audited.json)、[changsha-07-enterprises.audited.json](/Users/chenjunzhou/Documents/Codex/2026-08-27/x20/work/fuzhou-changsha-job-proof/public/data/changsha-07-enterprises.audited.json)。

OSM 标签的语义也说明它们只能产生候选：

- `office=*` 表示以服务或行政/专业工作为主的办公地点，不只包括私人公司；`name`、地址、联系方式和网站只是常见组合字段。[OSM `office` 文档](https://wiki.openstreetmap.org/wiki/Office)
- `craft=*` 表示生产或加工定制商品的作坊/从业者工作场所。[OSM `craft` 文档](https://wiki.openstreetmap.org/wiki/Craft)
- `man_made=works` 和 `industrial=*` 面向工厂、工业设施或工业类型，并不等价于工商主体。[OSM `industrial` 文档](https://wiki.openstreetmap.org/wiki/Key:industrial)

## 3. 直接城市查询能承诺什么

### 3.1 能承诺

对一次已声明规则版本的成功扫描，可以承诺：

- 已检查该城市行政边界范围内、当前 OSM 数据快照中命中声明过滤规则的全部对象；
- 每个候选保留 OSM 对象类型/ID、名称、位置、命中的标签和检查时间；
- 所有 bbox 分片、标签分支和分页/响应处理均完成后，该 **OSM 来源范围** 扫描完成；
- 零结果只表示该城市当前 OSM 快照没有命中规则的对象。

### 3.2 不能承诺

不能承诺：

- 城市工商登记企业全量或企业名录完整性；
- 企业登记状态、统一社会信用代码、母子公司关系或实际用工主体；
- OSM 名称一定是法定企业名，或地点一定是总部而非门店、分公司、园区楼栋、研究院；
- 企业有官网、招聘入口或当前岗位；
- OSM 未命中的企业在现实中不存在。

这些限制不是技术查询没写好，而是 OSM 作为协作地图的数据语义和覆盖率边界。将来若取得政府企业库或商业工商库的正式授权，应把它们作为并列 `CompanySource`；不能用来反向宣称 OSM 已经完整。

## 4. 公共服务的运行限制

公共 Overpass 实例适合一次性、小规模使用，不适合作为 Job Atlas 生产扫描的无条件依赖。OSM Wiki 当前给出的主实例建议是：一次性使用低于每天 10,000 次查询和 1 GB 下载通常不会干扰他人；长期、规律运行应将其缩小一百倍，即低于每天 100 次查询和 10 MB，并且不要并行运行多个脚本。页面还明确提醒公共实例可能过载、可靠性不高，商业使用应采用自托管或付费实例。[Overpass 公共实例与使用建议](https://wiki.openstreetmap.org/wiki/Overpass_API#Public_Overpass_API_instances)

Nominatim 公共服务也要求最多每秒一次、使用可识别的 User-Agent、缓存结果；系统化下载某区域全部 POI 被禁止，应使用 OSM planet/区域 extract 或自托管服务。[Nominatim Usage Policy](https://operations.osmfoundation.org/policies/nominatim/)

因此第一版 Adapter 应把端点做成配置，并遵守以下边界：

- Nominatim 只用于低频解析城市边界，结果持久缓存；不能用它下载城市企业/POI。
- 开发和低频验收可以节流使用公共 Overpass；不能并发轰炸公共实例。
- 规律扫描或数据量超过公共实例合理用量时，切换为自托管/付费 Overpass，或下载 OSM 区域 extract 后本地过滤；这只替换 Adapter 的传输实现，不改变业务模块。
- 公共端点超时、限流或响应未完整时，该已启用来源应进入现有重试/`blocked` 语义，不能把部分响应解释成扫描完成。

## 5. 对三个模块的修订建议

现有三个业务深模块仍然成立，但第一模块的输入和内部 Seam 应去除“必须先有地图区域/产业园”的假设。

建议改为：

```text
RegionalCompanyDiscovery.DiscoverCompanies(CityScope) -> CompanyInfo[]
```

其中 `CityScope` 是调用方声明的城市身份，不要求 `DiscoveryPlanner` 预先构造 `MapArea`。`RegionalCompanyDiscovery` 选择已启用的 `CompanySource`，由具体 Adapter 决定如何把城市转换成它能扫描的范围：

- `OSMCityCompanySource`：内部解析并缓存行政区边界，然后按 bbox 分片查询、按 polygon 裁切；
- 将来的政府企业数据 Adapter：按该来源支持的行政区代码、分页或数据集快照查询；
- 将来的商业企业数据 Adapter：只有在其授权和扫描边界真实满足要求时才启用。

公共 Seam 建议从 `AreaCompanySource` 改成更中性的 `CompanySource`，避免把所有来源强迫成地图查询。`CompanyInfo` 仍需包含名称、城市以及证明该候选来自哪个已启用来源的最小内部证据；官网发现仍属于 `OfficialJobDiscovery`，不移入第一模块。

`DiscoveryPlanner` 的第一步相应改成“把每个城市交给 `RegionalCompanyDiscovery`”，而不是“先把城市解析成地图区域”。地图边界解析是 OSM Adapter 的内部细节。

## 6. Ticket 14 的具体修订

建议删除原来的“从 OSM 地图区域发现当地公司/接入首个企业信息来源”模糊表述，改成：

### 14：从城市范围直接发现 OSM 企业候选

**Blocked by：** 12

**What it delivers：** 调用方提交一个城市后，Job Atlas 不经过产业园步骤，直接扫描该城市行政边界内命中已声明企业规则的 OSM 对象，并把完整候选增量转换成 `CompanyInfo`。每个候选保留 OSM 来源引用；任务只在所有范围分片和规则分支完成后把该来源标记为完成。

建议验收条件：

- 不提供任何产业园/载体输入，也能从城市产生 `CompanyInfo`；
- bbox 中落在真实行政区外的对象不会进入候选；
- 名称主体词和企业相关标签两类召回均受版本化规则控制，排除原因可审计；
- 同一 OSM 对象和可明确合并的重复对象不会重复投递；
- 超时、限流、分片失败可从检查点继续，未完成来源使任务进入既有重试/`blocked` 语义；
- 验收口径明确写成“OSM 中命中规则的企业候选”，不得写成“城市全部企业”；
- 福州、长沙既有文件只用于回归现有解析与审计规则，不重新采集，不作为城市全量验收数据。

产业园发现不再阻塞 Ticket 14，也不应成为 Ticket 15（官方招聘路径发现）的依赖。以后若要验证“产业园邻近启发式是否提高召回率”，应单独建立非阻塞研究/增强票据，而不是恢复为主流程的必经步骤。

