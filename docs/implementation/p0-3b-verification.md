# P0-3B / 阶段 1.2 实施与验收记录

日期：2026-09-23。分支：`dev`。实施起点：`2751007015450f2e94b321513ac505e3dfe828f5`
（设计文档提交），功能基线为已验收的 P0-3A `5f3aafa`。开始时工作区干净。
权威规格：[完整贡献类型](p0-3b-complete-contribution-types.md)。

## 实际交付

复用现有五个 Public、四个 Admin contribution endpoints、作者中心和审核队列，补齐：

| 类型 | 实际行为 |
| --- | --- |
| add_source | HTTP(S) URL 规范化/去重；作者不能设置版权、主来源或可用状态；审核确认可用状态，接受后 rights=unknown、非主来源 |
| remove_broken_source | 选择当前公开来源；接受仅设置 availability=removed，保留记录、版权及 primary，不回传被移除 URL |
| add_tag | 1～10 个现有有效标签，去重排序，只加不删；标签锁下复核资格，一次 bump，任一失败整份回滚 |
| add_relation | 固定两端及方向，同类型环检查；沿用 graph advisory lock、UUID 顺序锁、双端提交版本 CAS、双端审计 |
| add_translation | 非默认语言的原始行编辑；独立显示默认语言参考；省略、NULL、无行与显示回退不混用，支持部分修改 |

七种类型共用身份/已验证邮箱、额度、请求回放、撤回、拒绝、自审禁止、原稿与终态历史。
Editor/Admin 可修订后接受，内容修订必须说明原因；内部备注与作者消息隔离。
`ApplyReviewedOwnedTx` 在现有 curation 边界参与同一事务，不自行提交；canonical 修改、
accepted 快照、终态事件和每个 Resource 的版本审计一起提交或回滚。

私有详情和上下文用一致快照投影当前公开资格；返回前以 READ COMMITTED 再验当前授权，
防止快照早于持锁的会话/角色撤销。两个事务串行，不额外持有第二条连接；mutation 仍在单一
READ COMMITTED 事务内执行 User 锁、权限复核和 canonical CAS。

Public/Admin 生成类型和严格 decoder 共同限定 kind 对应 payload，拒绝混装、未知/只读字段。
旧两类的 content 契约保持兼容；新类型不用伪造完整基础资料。标签空基准明确表示尚未绑定。
Resource/Taxonomy 纯领域包、Auth、Redis/River、匿名 Astro SSR 的边界保持不变；无新增依赖。

## 浏览器验收

使用本机 disposable gfp_ci、虚构账号和临时 Resource graph；不使用或修改真实 Human Acceptance 账号。
实际操作 Public Web 与 Admin Web：

- 来源：提交带转义文本的原稿 → 审核修改标签及确认 availability → 采纳 → 作者查看原稿、修订说明及结果；内部备注不出现。
- 失效来源：从列表选择 → 提交 → 采纳 → 来源不再公开；本人历史仅显示移除动作，保留底层记录。
- 标签：选择两个标签 → 预览名称 → 采纳 → 单次版本递增，基准未绑定、当前与最终已绑定的对比正确。
- 关系：精确 slug 定位对端 → 明确方向预览 → 提交/采纳 → 双端分别 4→5、1→2，后台有两条版本记录，作者可查看结果。
- 翻译：新建 ja，目标文本初始为空，默认语言参考单独呈现；空摘要保留 NULL，脚本文字作为文本展示；采纳后作者原稿和结果正确。
- 冲突：Public 来源表单预览后，Admin 修改 Resource；提交返回版本冲突，URL/理由完整保留，没有自动重试或覆盖。
- Dirty guard：审核修订后尝试离开出现提示，选择保留继续编辑，输入未丢失；确认复选框不会清空预览。
- 截图检查审核对比与提交表单；Admin 控制台无 error。临时浏览器标签和服务已关闭。

拒绝/撤回覆盖五种新类型的真实数据库集成流程；旧两类的既有生命周期、额度、身份和并发回归全部继续执行。
未声称手动逐一重复七种类型的所有按钮组合。

## 数据库、权限与保留项

仅新增 `00008_complete_contribution_types.sql`，新增五张类型表：
`contribution_source_changes`、`contribution_tag_changes`、`contribution_relation_changes`、
`contribution_localization_changes`、`contribution_review_resource_changes`。

Goose 当前为 8，无额外迁移。迁移 1～7 内容及 SHA-256 与实施前一致；Resource Core
四角色逐对象/逐列权限及 ACL 指纹与基线完全相同。新 Contribution 表使用明确列授权，
API 无 canonical DML 或审计读取、Worker 无权限、Readonly 仅 SELECT；历史不可 UPDATE/DELETE。

Disposable 验证 Down 8 在存在新类型提议时拒绝且保持版本 8；清理测试自有新提议后，
8→7→8 保留旧提议及其原稿；随后执行旧 7→6→5 回归并恢复到 8。
共享 gfp_dev 只运行 migrate up，未运行 Down，未修改集群角色/Redis ACL 或管理共享服务器。

真实 Admin smoke 创建七个临时作者和一个审核账号，在现有流程中完成全部贡献类型，
校验公开结果、双端审计及权限，清理仅限本次夹具。没有绕过 60 秒提交额度。
既有私密配置的字节校验通过；未输出或跟踪凭据、token、DSN、真实 Tailnet 地址。

## 实际命令与结果

| 命令 / 操作 | 最终结果 |
| --- | --- |
| `git status --short`、`git branch --show-current`、`git log -1` | PASS，dev，起点工作区干净 |
| `pnpm generate` | PASS，Go/sqlc/Orval 产物生成 |
| `go -C server test ./internal/contribution ./internal/curation` | PASS |
| `go -C server test ./internal/transport/public ./internal/transport/admin ./internal/transport/contributionhttp` | PASS，普通测试；集成另行启用 |
| `pnpm typecheck` | PASS，最终包含在 check 中 |
| `go -C server test -count=1 -timeout=3m -run TestIntegrationContributionChanges ./internal/transport/public` | PASS，启用 CI/disposable/contribution guards；覆盖全部新类型 |
| `go -C server test -count=1 -timeout=3m -run TestIntegrationContributionChangesSnapshotRevoke ./internal/transport/public` | PASS，实际锁等待下验证角色/会话撤销 |
| `pnpm check` | PASS，审计、lint、类型、测试与构建 |
| `pnpm integration:ci` | PASS，`CI=true`、`GFP_DISPOSABLE_INFRA=1`，固定本机 disposable PostgreSQL/Redis；P0-1/P0-2/P0-3A、新类型、migration round-trip、SSR 全绿 |
| `pnpm build:images` | PASS，Server / Web / Admin / PostgreSQL 四个本地 runtime images |
| `pnpm migrate:dev` | PASS，Goose 8、官方 River up |
| `pnpm smoke:admin:dev` | PASS，真实权限/Auth/curation/七种贡献/公开结果与自有夹具清理 |
| `node .cache/p03b-grants.mjs` | PASS，本地只读审计 Goose 8 及 Resource ACL 指纹，辅助脚本不提交 |
| `node .cache/p03b-preserve.mjs` | PASS，迁移 1～7 和既有私密输入字节不变，辅助脚本不提交 |
| 最后 `pnpm generate`、`pnpm check:generated` | PASS，包含新增文件的生成产物逐字节一致，无 drift |
| `pnpm audit:repository`、`git diff --check`、staged diff 审查 | PASS，无 Secret/本地配置/真实地址被跟踪，无越界依赖或未来领域，完整改动与暂存内容已审查 |

开发过程中的失败均已定位：旧 schema/version 断言需要纳入迁移 8；重复使用已初始化的
CI 服务触发角色已存在，改为新建本次 disposable 容器；镜像初次构建遇本地代理未监听，
恢复本机 relay 后重跑；生成类型把 exists 标为只读后，表单改为显式构造提交对象；
并发测试改用可观察的 blocking PID，避免依赖测试角色无权读取的其他角色 SQL 文本。
标签基准展示和确认预览的交互问题也在浏览器验收中修复。
没有以修改共享权限、跳过校验或扩大架构来绕过问题。

## 范围与后续

实现范围止于阶段 1.2，未引入举报、信任/限制、来源巡检、搜索发现、收藏、通知、上传、
新 Redis key、River job 或未来 Domain 空包。阶段 1.3「举报与治理」仍需设计与实施。
阶段 1.2 可以关闭并进入 1.3 设计。完成本阶段不等于完整 P0 可上线；继续按照中文版路线图推进。
最终实现提交以本文件所在的 `feat: complete resource contribution types` 提交为准；仅本地 dev 提交，不 push。
