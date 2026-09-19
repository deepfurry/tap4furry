# P0-2B 实施与验收记录

执行日期：2026-09-19 至 2026-09-20（Asia/Shanghai）。分支：`dev`。
开始时工作区干净，HEAD 与 origin/dev 均为
`9c2995ce86ac0872d598da885925454b18607f79`。
依据：[P0-2B 规格](p0-2b-public-resource-read-surface.md)及当前 Agent/contracts。
附件入库仅规范化行尾空白，内容一致。实际审计没有发现实质架构冲突。

## 实现

新增四个匿名 Public GET：

| 路径 | operationId | 行为 |
| --- | --- | --- |
| `/resources` | `listResources` | 稳定分页、每页默认 24/最大 100、lookahead `has_next`，无 total |
| `/resources/{slug}` | `getResource` | 本地化详情、taxonomy、Sources、Relations、External IDs |
| `/categories` | `listCategories` | active 且未删除的 Category，按 slug 排序 |
| `/tags` | `listTags` | active 且未删除的 Tag，按 slug 排序 |

`public_resource.sql` 包含规格规定的九个具名查询。Handler 通过既有 API pool
直接使用 sqlc；Resource/Taxonomy domain 保持不变。详情在一个只读
repeatable-read 事务中组装，避免多条查询看到不同的可见性状态。
新路径具有五秒 deadline，位于 Origin/CSRF/OAuth/session 边界之外。
集成测试用 nil Auth/Identity/Health、无 Redis 的真实 Handler 验证独立性。

- Resource 仅 published、未删除且 Category 未删除时公开；所有 lifecycle
  均可公开，published explicit/discontinued 实际返回 200。
- Browse 不返回 retired/deleted taxonomy；公开 Resource 可关联 retired
  Category/Tag，deleted Category 隐藏 Resource，deleted Tag 不进入响应。
- Source 在 SQL 中过滤 availability 与 rights：30 种组合中只有允许的 12 种
  返回。公开 DTO 不包含 rights_status，也不包含内部归属/时间列。
- Relation 只返回公开对端；四个存储类型保留，direction 使用
  outgoing/incoming/symmetric，不返回关系行 UUID 或隐藏 Resource 的信息。
- Locale 使用既有 taxonomy.ParseLocale；省略时每个实体使用自身默认翻译。
  requested/default 按字段独立回退，不做语言族推断。测试覆盖 requested name、
  requested summary/description 分别与默认字段组合、缺失翻译回退、显式 null。
- LEFT JOIN 保留缺失 canonical localization 的异常行，显式检查后返回
  `500 INTERNAL_ERROR`。普通隐藏/不存在/不合法 slug 统一为
  `404 RESOURCE_NOT_FOUND`；参数错误为 `400 VALIDATION_ERROR`。
- 实际序列化响应递归检查没有 publication_state、version、deleted_at、
  rights_status、created_at、resource_id；详情、列表、Source、Relation/ref
  另检查准确字段集合。空列表返回 `[]`。

## Astro 与 Markdown

新增 `/resources`、`/resources/[slug]` 和 PublicLayout，纯 Astro SSR。
实际 HTML 的 Resource React island 数为 **0**，没有 hydration script。
页面显示列表、详情、状态/内容分级、Sources/Relations/External IDs，没有写操作。

`public-api.server.ts` 复用 generated URL builders，仅去掉开头 `/api`，直接访问
server-only `API_INTERNAL_ORIGIN`。开发可默认 loopback，production 必须明确配置。
五秒超时、无重试；fake API 记录证实 SSR 没有转发 Cookie/Authorization/CSRF。
Web 默认显式请求 en；JA/zh-hans 被规范化，locale 保留在详情及分页链接中。

Markdown-it 15.0.2（自带类型）与 sanitize-html 2.17.7 仅加入 Web；增加必要的
`@types/sanitize-html` 2.16.1，没有新增 Go 依赖或 React Markdown/UI 栈。
raw HTML/linkify/typographer 禁用；图片 renderer 不产生输出，sanitize allowlist
只允许规定的文本元素与安全链接，外链具有 `ugc nofollow noreferrer`。
没有 MDX/custom components、img、危险协议链接或强制新窗口。
Markdown.astro 是唯一 `set:html`；审计与反例测试阻止额外 sink、客户端直接/
动态导入及转导出 server helper，并禁止 Resource 页面 hydration。

SSR 验收使用本机 fake API，与共享 Infra 无关，验证：

- SSR 列表/详情正文、空列表、上一页/下一页与 locale 链接；
- title、summary/fallback meta description、Open Graph、html lang；
- canonical 固定为生产站点，排除 locale，列表仅 page > 1 保留 page；
- 成功响应共享缓存头；400/404/503 为 no-store/noindex，没有 Accept-Language Vary；
- API 400/404 状态保留；5xx、非法 JSON/shape、redirect 和超时转为安全 503；
- HTML/脚本/编码危险链接/XSS、Markdown 图片、h1 降级、允许的表格/代码/文本；
- HTML 中没有内部 origin、上游错误内容、图片或 astro-island。

## 数据库与 shared dev

本阶段没有执行共享 migration up/down，没有改变 schema、索引、权限或角色。
开始和结束只读审计均为 Goose **6**、10 张 Resource Core 表、无 vector。
`pnpm smoke:public-read:dev` 使用现有 API/migrator 私有配置与真实 pgx/Handler：
四个 endpoints、visibility、locale、Source/Relation privacy、分页、DTO 与缓存
全部 PASS；migrator 按 FK 安全顺序只清理本次随机 UUIDv7 fixture。
结束后 Resource Core 总行数回到 **0**。没有触碰现有账号或身份数据。

00001～00006 的 SHA-256 与开始前全部相同，不存在 00007；7 个既有私有配置
也逐字节未变。Go module/sum、resource/taxonomy domain、Admin 应用/API 未改变。
没有新增 Redis key/cache、River job、Worker Resource 权限或业务。
disposable CI 保留 P0-2A 的受保护 6→5→6 round-trip；共享环境没有 down。

## 实际执行命令及结果

| 命令 | 最终结果 |
| --- | --- |
| `pnpm check`（基线及最终实现） | PASS；审计、生成 drift、ESLint/gofmt/vet、类型检查、Node/Go 测试和应用构建 |
| `pnpm generate` | PASS；sqlc、oapi-codegen、Orval 输出提交，未手改 generated files |
| `pnpm check:generated`（check 后再次生成） | PASS；字节及文件集合均无 drift |
| `go -C server test ./internal/transport/public ./cmd/api` | PASS |
| `go -C server test ./internal/transport/public ./internal/database/publicreadcheck ./cmd/public-read-smoke` | PASS；数据库集成在显式 CI 下另行执行 |
| `go -C server test -count=1 -timeout=3m -run TestIntegrationPublic ./internal/transport/public` | PASS；CI/disposable/public-read 三个 guards 均开启 |
| `pnpm exec eslint .` | PASS |
| `pnpm --filter @tap4furry/web typecheck` | PASS |
| `pnpm --filter @tap4furry/web build` | PASS |
| `node scripts/ssr-public-read.mjs` | PASS；也在全量 integration:ci 中再次通过 |
| `$env:CI='true'; $env:GFP_DISPOSABLE_INFRA='1'; pnpm integration:ci` | PASS；最终从新建 PostgreSQL 18 / Redis 8 执行全部 P0-1/P0-2A/P0-2B 回归 |
| `pnpm build:images` | PASS；Server/Web/Admin/PostgreSQL 四镜像，无发布 |
| `pnpm smoke:public-read:dev` | PASS；共享 dev 真实读取及 fixture 清理 |
| `pnpm audit:repository` | PASS；Secret/Tailnet、依赖、运行边界与 Markdown/server-only 审计 |
| `git diff --check`、`git diff --cached --check` | PASS |
| `node .cache/p02a-db-post-audit.mjs`（复用已有只读审计） | PASS；开始/结束确认身份、Goose 6、10 表及结束清理 |
| `node .cache/p02b-preservation-check.mjs` | PASS；旧 migration/私有输入 hash、冻结代码、spec 与 dev/main |

依赖安装实际使用 `pnpm --filter @tap4furry/web add --save-exact markdown-it@15.0.2 sanitize-html@2.17.7`
和 `pnpm --filter @tap4furry/web add -D --save-exact @types/sanitize-html@2.16.1`，均成功。
锁文件只增加这些依赖及其传递依赖。Ignored `.cache/p02b-*.log` 保留详细本地日志；
只读辅助脚本和私有配置均不提交。

初次验证发现并已解决：

1. sqlc 对 Relation CTE 的未限定列报告歧义；显式 outgoing/incoming alias 后生成通过。
2. Go DTO 字段测试复用 map，JSON 解码保留前一详情的键；重置 map 后确认真实响应
   无泄漏，目标测试和全新 disposable 全量 CI 均 PASS。
3. SSR 的初版测试将 Astro 自身的样式标签误计入 Markdown 禁止元素；改为检查
   Markdown 输出边界，同时保留全页面 script/island/img 检查，完整 SSR 验收 PASS。
4. 镜像包下载有一次连接重置，包管理器自动重试后四镜像构建成功。

最终完整 diff/暂存区与依赖已复查，未跟踪 Secret、私有配置或真实 Tailnet 地址。
没有 Admin CRUD、Contribution、Search/Discovery、Collection、Exchange、Media
或其他 P0-2C/P0-4 工作。P0-2B gate 全部通过，可以进入
**P0-2C — Admin Resource Curation**。只在 dev 本地提交，不 push/merge main/tag/release/部署。
