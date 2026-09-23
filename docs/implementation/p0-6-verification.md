# P0-6 / 阶段 1.3 验收记录

日期：2026-09-24。实施基线：`dev` / `1c2d7e416785bee6fe258569be5e84ee4a64e4b6`。
依据：[举报与治理规格](p0-6-reports-governance.md)、[设计前审计](p0-6-baseline-audit.md)。

## 实际交付

- Public：资源／来源私密举报、预览确认、本人列表与处理历史、撤回；账号页展示有效限制与实际额度。
- Admin：举报队列、受理／转交／内部核查／解决／驳回，用户范围限制与信任档位，来源人工核查，业务审计。
  推荐排除和高影响操作理由整合既有 Resource／Taxonomy 编辑页。
- `internal/governance` 提供固定政策及事务内审计；`internal/moderation` 编排 Auth、Identity、Curation。
  Resource／Taxonomy 仍为纯领域，未引入未来业务包、依赖、Redis key、River job 或 URL 抓取。
- 所有实际 canonical mutation（含贡献接受）原子写审计；复合举报结案只允许 publication、
  Source availability、Source rights、distribution 四类动作。CAS stale／审计失败完整回滚，no-op 不 bump／造审计。
- 用户治理先按 UUID 顺序锁 actor 与 target，再复核当前 session/capability；业务写入锁同一 User。
  修复基线审计 P2-01：资料写入经 moderation 事务授权后调用 `identity.ApplyProfileTx`，无旧运行入口旁路。
- all_write 只阻止贡献提交和公开资料编辑。登录、账号安全、举报、本人记录／撤回和关闭索引保持可用。
  三档贡献额度为 10/30/60 每日、5/10/20 待审、60/30/15 秒间隔；举报独立 10/5/60。
- 展示与推荐资格分开；unknown rights 可展示但不能贡献推荐资格。来源核查绑定 URL 指纹及 Resource 版本，
  不自动恢复 availability／rights。四个匿名 read endpoints 和 Resource SSR 均改为 no-store。

## Migration 与权限

新增 `00009_governance_foundation.sql`，八张表为：`reports`、`report_events`、
`user_governance_profiles`、`user_restrictions`、`resource_distribution_policies`、
`source_checks`、`moderation_actions`、`audit_entries`。

Goose 已在共享 `gfp_dev` 从 8 向上升级到 9；没有执行 shared Down。
迁移 1～8 与实施前 SHA-256 比较 PASS。身份、Resource、Contribution 的既有表／列 ACL
与升级前指纹完全相同，Resource 四角色的实际权限矩阵另行通过检查。
新表逐列验证 API／Admin 最小读写、Readonly 只读、Worker 无权限；历史／审计不授予 runtime UPDATE/DELETE。

disposable `gfp_ci` 证明：有治理数据时 Down 9 拒绝且版本仍为 9；清理本次夹具后 9→8→9 成功，
七种旧贡献及来源／关系／翻译 typed snapshots 字节摘要保持一致。继续执行 8／7／6 的既有保护与往返测试。

## 实际执行命令

所有数据库集成测试使用固定 loopback disposable PostgreSQL／Redis，未读取共享私有配置。
Windows 镜像构建使用本机既有隔离 WSL Docker 和本地验证 relay，没有管理共享 Infra。

| 命令／检查 | 最终结果 |
| --- | --- |
| `pnpm generate` | PASS，OpenAPI／sqlc／Go／TypeScript 链路 |
| `pnpm check` | PASS，仓库审计、生成 drift、ESLint、gofmt、go vet、前端类型、Node/Go tests、全应用构建 |
| `pnpm integration:ci` | PASS，fresh migrations／重复 up／往返、P0-1／P0-2／P0-3／P0-6 回归、角色 driver smoke、River、SSR |
| `pnpm build:images` | PASS，Server／Web／Admin／PostgreSQL 四个 runtime images |
| `pnpm migrate:dev` | PASS，Goose 9 与官方 River up；只向上迁移 |
| `pnpm smoke:admin:dev` | PASS，真实 Admin／Public／owner 边界、七种贡献及新治理闭环；仅清理本次临时夹具 |
| `pnpm generate` 后 `pnpm check:generated` | PASS，逐字节一致，包括文件新增／删除检测 |
| `pnpm audit:repository`、`git diff --check`、staged diff 检查 | PASS |
| `node .cache/p06-preserve-check.mjs` | PASS，迁移 1～8 与既有私有输入哈希相同，不输出私密值 |
| `node .cache/p06-old-acl.mjs`、`node .cache/p06-core-grants.mjs` | PASS，既有 ACL 指纹及 Resource 实际权限保持 |
| `node .cache/p06-recover-clean.mjs`、`node .cache/p06-clean-audit.mjs` | PASS，按确切 ID 原子清理首次中断的本次夹具；共享治理表无遗留数据，既有 ACL 保持 |

开发期间另外运行过相关 `go -C server test` 子集，包括完整 `TestIntegrationGovernance`，最终均 PASS。
早期迭代发现并修正了旧 schema 白名单、OAuth 测试／Auth smoke 的数据库池注入以及旧 Public 缓存断言。
一次测试启动错误地让 migration 往返与 HTTP 测试包并行，已改为现有 CI 的顺序验证并重跑通过。
镜像首次失败于本地 relay 未启动，另一次为本地 Docker bridge 丢失；恢复本地验证环境后完整重跑通过。
这些失败不计为通过，也未据此放宽产品权限或更改共享基础设施。
首次共享 smoke 被会话中断，重跑完整通过后，按创建时间、固定夹具内容及关联 ID 复核并原子清理
首次遗留的 8 个临时账号、4 份举报和 1 组 Resource／Category；没有处理既有 Human Acceptance 账号。

## 关键行为证据

- 举报：Source 归属、不可见安全 404、canonical 损坏、4,000 Unicode 上限、原稿／内部 DTO 隔离、
  当前公开链接、回放／键冲突、同目标重复、独立日限／待处理／间隔、分页及终态竞争。
- 角色：Moderator 处理普通问题；敏感类别不能越权裁定；Editor 无原始举报／全局审计权限；
  Admin 才能治理用户／rights／分发，禁止自审与自我限制。等待锁期间撤销角色／session 会拒绝操作。
- 限制／额度：三档调整和降级、缺行默认、CAS／no-op、显式替换留两条历史、撤销／到期、
  all_write 下七种贡献均受控、成功旧请求可回放、混合资料请求不部分写入、隐私 opt-out 保留。
- 并发：建立限制与资料写入串行；等待锁期间撤销／到期使用新状态与锁后 DB 时间；两名 Admin
  相互治理不产生反向 User 锁；举报决定／撤回仅一个终态；审计阻塞并取消导致整份复合操作回滚。
- 来源／资格：观察无 Resource bump、重复请求不重复记、stale 拒绝、URL 变化使旧核查非当前，
  权利封锁不能被 availability 恢复绕过；纯资格规则与 PostgreSQL 候选判断一致。
- 既有回归：贡献修订接受／双端关系 CAS／图环竞争／作者投影、Admin→Public 生命周期、
  Astro XSS／unsafe link／无图片／无 islands／SEO／no-store／status／timeout 及无凭据转发通过。

## 实际浏览器闭环

使用 disposable 数据库的临时作者和 Admin，通过真实 Public Web／Admin Web 页面完成：

1. 从匿名 Resource 的来源举报链接进入私密表单，预览纯文本原稿并提交。
2. 后台受理；填写内部备注后切换路由触发 dirty guard，选择继续编辑保留输入。
3. 预览作者可见消息，显式移除 Source 并解决举报；作者页只显示原稿、安全消息和状态，内部备注不可见。
4. Public 资源立即不再显示该 Source；元数据仍可访问。
5. 对临时作者建立 all_write，账号页资料表单禁用，安全／会话入口保留，关闭索引成功。
6. 撤销限制后，作者资料编辑恢复并实际保存成功。
7. 人工记录 reachable 并显式更新 availability=active，Resource 版本前进，Public 来源重新可见。
8. 查看审计时间线，核对 source 的版本、前后 availability、actor 和理由；页面布局已截图检查。

## 收口与下一步

阶段 1.3 和阶段一可关闭，下一步进入 **2.1 搜索与发现** 的设计／实施。
本阶段没有提前实现 Search／Collection，没有自动处罚、自动信任升级、巡检或通知系统。
未 push、未 merge main、未 tag/release、未部署生产。提交 SHA 由最终提交结果给出，避免文档自引用。
后续生产部署如存在旧共享缓存，须先等待过期或清除；no-store 不能追回已加载内容。
