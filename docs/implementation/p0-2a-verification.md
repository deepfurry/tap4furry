# P0-2A 实施与验收记录

日期：2026-09-19。分支：`dev`。
开始时工作区干净，HEAD 为
`66c8abd556262b0a480be50d0b88905dd1ca86d5`（MAIL-0）。
实施依据：[P0-2A 规格](p0-2a-resource-domain-schema.md)与当前工程契约。
附件入库仅规范化行尾空白，内容一致。

## 实现范围

- 新增 `00006_resource_core.sql`，在 `app` schema 建立以下 10 张表：
  `categories`、`category_localizations`、`tags`、`tag_localizations`、
  `resources`、`resource_localizations`、`resource_tags`、`resource_sources`、
  `resource_relations`、`resource_external_ids`。
- 明确 UUID、文本/状态/时间约束、唯一性、RESTRICT 外键与所需索引。
  不建立 Category seed、trigger、RLS、PostgreSQL ENUM、JSONB/EAV。
- `taxonomy` 提供平面分类状态、slug、Locale 与本地化文本规则；
  `resource` 提供相互独立的发布状态、生命周期、内容分级及基本不变量。
- Locale 用已有 `golang.org/x/text/language` 解析和规范化；文本按 Unicode
  字符计数，空可选文本归一为 nil，Resource description 不设小长度上限。
  数据库拒绝同一实体仅大小写不同的 locale。事务测试证明实体与默认翻译
  原子创建、默认语言切换必须已有目标翻译、禁止删除当前默认翻译。
- Source 的 type / availability / rights 独立；URL 保守规范化，保留路径转义、
  query 顺序及内容，拒绝 userinfo 和非 HTTP(S)，不访问 URL。每个 Resource
  至多一个 primary Source；不同 Resource 可以使用相同 URL。
- 四种 Relation 类型；有向关系保留方向，`related_to` 规范化 UUID 顺序，
  禁止自关联和重复逆向存储。External ID 的 namespace/value 全局唯一。
- sqlc 提供存在性/状态查询、父行锁和 version CAS。并发测试验证恰好一个
  写入成功、一个冲突；旧版本事务不泄漏子项改动。多语句逻辑变更只加一次
  version，关系变更两端各加一次；独立 taxonomy 改动不级联增加版本。
  soft-deleted Resource 不进入普通读取或 CAS。
- 新增 `smoke:resource:dev`，使用五个准备好的真实身份；只有 migrator 清理
  本次随机 UUIDv7 fixture。验收辅助包不会进入 API/Admin/Worker 运行依赖。
- 更新 Agent 路由、contracts、产品/架构/开发文档及 CHANGELOG。
  x/text 仅从间接依赖提升为直接依赖，仍为 v0.41.0；锁文件未变化。

没有实现 Public/Admin Resource HTTP API、Resource 前端、完整 CRUD、
Contribution、Search、Collection、Exchange 或其他后续业务。
没有新增 Redis key、River job 或 Worker 业务，也没有改变 Infra 标识。

## 数据库与权限

开始前用实际五个角色做只读审计：共享数据库为 `gfp_dev`、Goose 版本 5、
Resource Core 对象数为 0、migrator 有既有的 app CREATE 能力、没有 vector。
未发现要求改变架构或扩大共享 Infra 权限的实质冲突。

| 身份 | disposable | shared dev | 实际检查 |
| --- | --- | --- | --- |
| `gfp_api` | PASS | PASS | 全部 10 表 SELECT；INSERT/UPDATE/DELETE 拒绝 |
| `gfp_admin` | PASS | PASS | 全部 10 表 INSERT/SELECT、精确列级 UPDATE 和指定子表 DELETE；禁止硬删父实体/Source、禁止更新 taxonomy slug、身份列和 created_at |
| `gfp_worker` | PASS | PASS | 全部 Resource Core SELECT/DML 拒绝 |
| `gfp_readonly` | PASS | PASS | SELECT 成功，DML 拒绝 |

检查有效权限与实际 SQL；禁止操作要求 SQLSTATE 42501，并检查没有
TRUNCATE/REFERENCES/TRIGGER/MAINTAIN 或额外列 UPDATE 权限。
没有修改共享角色、成员关系、schema 权限或默认授权，只由 migration
明确设置新增 10 张表的对象权限。

disposable PostgreSQL 18 上完成 `00001..00006 Up → 00006 Down → 00006 Up`：
版本 `6 → 5 → 6`，Down 后 Resource 表全部消失、P0-1 表保留；重新 Up 无种子。
Down 仅存在于测试代码，要求 `CI=true`、`GFP_DISPOSABLE_INFRA=1`、
固定 loopback `gfp_ci` 与实际 migrator 身份，并要求当前/目标版本恰为 6。

共享 `gfp_dev` 只执行 up。最终只读复查为 Goose 版本 6、10 张 Resource Core
表、合计 0 条 Resource Core 行；smoke 临时数据已完全清理。没有执行共享 down。
00001～00005 的 SHA-256 与开始前全部相同；7 个既有私有输入文件也逐字节未变。

## 实际执行的验证

工具版本：Go 1.27.1、Node v24.15.0、pnpm 10.11.0。
镜像和 disposable 服务使用本机专用 WSL Docker；CI 不读取任何开发私有配置，
不连接共享 Infra，也不调用真实邮件或 OAuth provider。

| 命令 | 最终结果 |
| --- | --- |
| `pnpm check`（开始前基线、实现后及 fixture 修正后） | PASS；审计、生成 drift、lint/vet、类型检查、测试及应用构建 |
| `go -C server mod tidy` | PASS；仅提升既有 x/text 为直接依赖 |
| `go -C server test -count=1 ./internal/taxonomy ./internal/resource ./internal/database/migrate` | PASS；disposable 测试在普通单测中按 guard 跳过 |
| `go -C server test ./internal/database/resourcecheck ./cmd/resource-smoke` | PASS；数据库集成另在 CI 中实际执行 |
| `pnpm exec eslint .` | PASS |
| `pnpm generate` | PASS；生成 sqlc，OpenAPI/客户端契约未变化 |
| `pnpm check:generated`（完整 check 后再次执行） | PASS；输出字节及文件集合无 drift |
| `$env:CI='true'; $env:GFP_DISPOSABLE_INFRA='1'; pnpm integration:ci` | PASS；最终在新建的 PostgreSQL 18 / Redis 8 上重跑 |
| `pnpm build:images` | PASS；最终代码的 Server/Web/Admin/PostgreSQL 四个镜像 |
| `pnpm migrate:dev` | PASS；共享库仅 up 到 6 |
| `pnpm smoke:resource:dev` | PASS；真实 pgx、四角色、fixture/CAS 与 owner 清理 |
| `pnpm audit:repository` | PASS；Secret、Tailnet、依赖及边界检查 |
| `git diff --check`、`git diff --cached --check` | PASS |
| `node .cache/p02a-db-audit.mjs` | PASS；实施前只读实际数据库审计 |
| `node .cache/p02a-db-post-audit.mjs` | PASS；实施后身份、版本、表数与清理复核 |
| `node .cache/p02a-preservation-check.mjs` | PASS；旧 migration/私有输入 hash、spec 内容、dev/main 检查 |

最后三个脚本为 ignored 本地只读验收辅助文件，不随提交发布、不包含私有凭据。
完整日志仅保留在 ignored `.cache/p02a-*.log`，报告不包含连接值或敏感内容。

## 验证中发现并修正的问题

1. 初次全量 CI 的旧 P0-1 测试仍精确要求版本 5 和 9 张表。已更新为版本 6
   和明确列出的 19 张表，继续拒绝未知表；旧鉴权/权限断言保持。
2. RESTRICT 删除测试实际返回 SQLSTATE 23001，已修正对应预期；不存在的外键
   仍验证 23503。disposable Worker 无 app USAGE，按名称检查表权限会先失败；
   改用 pg_class OID 检查有效权限，未增加 Worker 的 schema 或表权限。
3. 首次共享 Resource smoke 返回 23514，fixture 清理成功。只读诊断发现共享
   数据库时钟比本机约慢 0.18 秒，fixture 创建用本机时间而更新用数据库时间。
   现统一用数据库事务时间；重跑 shared smoke、完整 check、全新 disposable CI、
   再生成/drift 及四镜像构建均 PASS。没有修改时间约束或产品逻辑来绕过检查。

最终实现和生成结果已逐文件复查，未跟踪私有配置、Secret 或真实 Tailnet 地址。
所有 P0-2A 验收 gate 通过，可以进入 **P0-2B — Public Resource Read Surface**。
本次只在 dev 本地提交；没有 push、merge main、tag/release 或部署。
