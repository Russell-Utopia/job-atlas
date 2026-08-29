# Issue tracker：GitHub Issues

本仓库以 [GitHub Issues](https://github.com/Russell-Utopia/job-atlas/issues) 作为议题的唯一事实来源；不要在 `.scratch/` 下保存重复的 Issue 文件。

## 约定

- 已收敛的设计讨论使用 `decision` 标签并关闭；完整问题、讨论和答案保留在 Issue 正文中。
- 待实施任务保持开放，并使用 `ready-for-agent`、`ready-for-human`、`needs-info`、`needs-triage` 或 `wontfix` 中适用的标签。`ready-for-agent` 只表示规格完整，不表示依赖已经解除。
- GitHub 原生 Issue dependency 是依赖关系的权威来源。正文中的 `Blocked by: #<issue-number>` 是便于阅读的镜像，两者必须一致。
- 可执行前沿是“仍开放、未被领取，且 `issue_dependencies_summary.blocked_by == 0`”的任务；不要把所有开放的 `ready-for-agent` Issue 当作当前前沿。
- 领取任务时在 Issue 中留言说明；完成后附上验证结果并关闭 Issue。
- `.scratch/job-atlas/map.md` 继续保存设计地图，研究材料继续保存在 `.scratch/job-atlas/research/`，但其中的议题链接必须指向 GitHub。

## 创建或修改依赖

1. 先创建或更新 Issue 正文中的 `Blocked by`，再为每条边创建 GitHub 原生依赖。
2. 取得前置 Issue 的数字数据库 ID：

   ```bash
   gh api repos/Russell-Utopia/job-atlas/issues/<blocker-number> --jq .id
   ```

3. 把前置关系写到被阻塞的 Issue；`issue_id` 使用上一步的数据库 ID，不是 Issue 编号或 `node_id`：

   ```bash
   gh api --method POST \
     repos/Russell-Utopia/job-atlas/issues/<child-number>/dependencies/blocked_by \
     -F issue_id=<blocker-database-id>
   ```

4. 发布或调整完成前，回读每张子 Issue 的全部原生前置关系，并与正文逐条核对：

   ```bash
   gh api \
     repos/Russell-Utopia/job-atlas/issues/<child-number>/dependencies/blocked_by \
     --jq '[.[].number] | sort'

   gh api \
     repos/Russell-Utopia/job-atlas/issues/<child-number> \
     --jq .issue_dependencies_summary
   ```

只有正文与原生依赖完全一致，依赖发布才算完成。若仓库不支持原生 dependencies，才退回正文 `Blocked by`，并在领取任务前逐一检查前置 Issue 状态。

## 当前任务入口

- [规格完整的开放任务](https://github.com/Russell-Utopia/job-atlas/issues?q=is%3Aissue+is%3Aopen+label%3Aready-for-agent)（再按原生依赖筛选可执行前沿）
- [已关闭的设计决策](https://github.com/Russell-Utopia/job-atlas/issues?q=is%3Aissue+is%3Aclosed+label%3Adecision)
