# 阶段 1.1 / P0-3A 验收记录

日期：2026-09-23。分支：`dev`。实施起点：`5f45d83`（阶段 1.1 设计），功能基线：`96ef1d5`（P0-2C）。
结论：**基础贡献审核已完成，可以进入阶段 1.2。** 本次没有 push、合并 main、发布或部署。

## 实际交付

- 新资源提议和已有公开资源默认语言基础资料修改；名称、简介、说明、分类、生命周期与内容分级。
- Public 提交、编辑上下文、本人列表/详情/撤回；Admin 队列/详情/接受/拒绝；OpenAPI、Go 和 TypeScript 客户端同步生成。
- Public `/submit`、`/me/contributions`、`/contributions/[id]`；Admin `/contributions`、`/contributions/$contributionId`，接入现有导航。
- 审核前后内容对比、必填修订/拒绝原因、私密备注、显式预览和确认、dirty guard、冲突保留输入及成功后 refetch。
- `internal/contribution` 负责事务；通过 `curation.ApplyReviewedTx` 原子写入 Resource、采纳快照、事件和业务审计。新资源为 version 1 草稿，已有资源严格 CAS 且保持发布状态。
- PostgreSQL 内的 10 次/滚动 24 小时、5 条待审、60 秒间隔限制；同 request_id 同内容重放返回同一记录，异体请求冲突。

## 数据库与隐私

迁移 `00007_contribution_review.sql` 建立五张表：

| 表 | 用途 |
| --- | --- |
| `app.contributions` | 归属、状态、基准版本、请求摘要、实际提交字段、结果引用 |
| `app.contribution_contents` | 类型化 base/proposed/accepted 快照 |
| `app.contribution_initial_sources` | 新资源的单个初始来源 |
| `app.contribution_events` | 提交、接受、拒绝、撤回及分离的作者说明/内部备注 |
| `app.contribution_review_audits` | 同事务审核动作、操作者与前后 Resource 版本 |

用户/会话在事务内重验，审核还重验实时 Editorial 并禁止自审。锁顺序为 actor User → proposal → canonical parents。
任何 CAS、约束或审计失败都完整回滚。终态唯一；不自动合并、重放旧快照或修改原稿。

作者历史只输出本人实际提供的字段，保留显式 null，排除服务端补齐的基准字段。
隐藏资源不输出目标/结果链接或审核新增内容；内部备注、审核者 ID、版本和治理字段只在审核端出现。
采纳快照只在结果当前公开且版本仍与采纳版本一致时供作者对比，后续版本只链接当前公开资源，防止历史内容绕过治理。

权限逐列验证通过：API 仍不能写 Resource Core，仅有贡献提交/撤回所需权限；Admin 获得贡献审核最小 DML；Worker 无贡献权限；Readonly 只读。
Resource Core 四个角色的实际 ACL 指纹与实施前一致。迁移 00001～00006 与原有私密配置逐文件 SHA-256 比较一致；没有新增依赖。

## 执行命令与结果

| 实际执行 | 最终结果 |
| --- | --- |
| `pnpm generate` | PASS，Go/sqlc/Orval 生成成功 |
| `pnpm check` | PASS，仓库边界/秘密审计、生成检查、ESLint/gofmt/vet、TS/Astro、19 项 Node 测试、Go 单测和全部应用构建 |
| `pnpm integration:ci`（进程环境 `CI=true`、`GFP_DISPOSABLE_INFRA=1`） | PASS，全新一次性 PostgreSQL 18 / Redis 8；P0-1、P0-2A/B/C 和 Contribution 回归 |
| `go -C server test -count=1 -timeout=3m -run TestIntegrationContribution ./internal/transport/public`（另设 `GFP_CONTRIBUTION_INTEGRATION=1`） | PASS，针对性真实驱动回归 |
| `go -C server test ./internal/contribution ./internal/transport/contributionhttp` | PASS，Unicode/URL/字段语义、基准 MAC、额度边界与严格 JSON |
| `pnpm build:images` | PASS，Server / Web / Admin / PostgreSQL 四个镜像构建成功 |
| `pnpm migrate:dev` | PASS，共享 `gfp_dev` 仅 Up 到 Goose 7，官方 River 迁移路径幂等执行 |
| `pnpm smoke:admin:dev` | PASS，既有认证/curation 验收，以及真实提交、幂等重放、修订采纳、草稿发布、纠错、作者隔离与审计；只清理本次临时夹具 |
| `pnpm check:generated` | PASS，再次执行 `pnpm generate` 后生成文件字节一致，无新增/删除 drift |
| `pnpm audit:repository` | PASS，无秘密、真实 Tailnet 地址、非法依赖或边界违规 |
| `node .cache/p03a-preservation-check.mjs` | PASS，本地只读校验 1～6 迁移及既有私密文件，未输出文件内容 |
| `node .cache/p03a-grants.mjs` | PASS，Goose 7 与 Resource Core 四角色精确授权保持不变 |
| `git diff --check`、完整差异与暂存差异审阅 | PASS |

`.cache` 中的两个 preservation/grants 辅助脚本为本次本地审计材料，不提交；可重复的权限测试位于 `contributioncheck` 和既有 `resourcecheck`，由正式验收命令执行。

新增集成覆盖：未验证提交/越权读取/Moderator/自审/角色撤销/会话失效；并发提交额度和幂等；两个审核者及接受/撤回竞争；旧版本/默认语言变化/退休分类/no-op/slug 冲突；故障注入后的资源、采纳内容、事件与审计回滚；字段级作者投影、显式清空、隐藏目标与内部备注隔离。

一次性迁移验证执行 **7→6→5→7**，Down 7 保留 Resource Core，Down 6 保留 Auth，重新 Up 不产生种子数据。共享环境从未执行 Down。
SSR 验证保留 Markdown XSS/unsafe URL/无图片/无 Resource islands/SEO/cache/status 检查，并验证三个私有页面 no-store/noindex、无服务端凭据代理。

## 浏览器实操

使用一次性 `gfp_ci` 的独立作者和 Editor，操作真实页面与 Go API：

1. 创建临时分类 → 作者表单提交新资源 → 本人原稿与审核队列均出现。
2. Editor 改名并填写修订原因、私密备注 → 尝试离开触发 dirty guard → 保留输入 → 预览标出修改 → 采纳为草稿。
3. 作者仍见原稿和安全说明，不见草稿结果或内部备注；在既有 Resource editor 发布后，作者可访问公开结果。
4. 从公开页“Suggest a correction”进入默认语言表单 → 修改简介 → 审核原样接受 → 作者看到当前采纳内容。
5. 原稿与审核预览中的 `<script>` 均为文本；公开页由既有 Markdown 安全渲染。检查前后台布局及分类名称呈现。

临时浏览器页和本次启动的应用进程已关闭；浏览器夹具随一次性容器重建清除，不触碰原 Human Acceptance 账号。

本地 Docker Desktop 未能启动，复用已有专用 WSL 验证环境。修复了临时代理配置失效和 Redis 重启后的测试 ACL，随后四镜像和全部 CI 成功。
早期检查发现的旧版本断言、测试夹具字段名和 lint 问题均已修正；最终命令如上全部通过，未调整产品架构或降低安全校验。

## 后续范围

下一步为 **1.2 完整贡献类型**：已有资源来源、标签、关系和翻译提议。举报治理在 1.3，搜索/发现、收藏与完整产品界面在阶段二。
本次未实现这些后续能力，也未新增业务 Worker/River job、Redis key、邮件通知、富文本或通用工作流框架。
阶段 1.1 完成不代表完整 P0 或生产上线已经完成。
