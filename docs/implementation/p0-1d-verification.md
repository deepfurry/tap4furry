# P0-1D 实施与验收报告

验收日期：2026-09-09。实施依据为
[P0-1D 规格](p0-1d-admin-auth-roles-hardening.md)及仓库工程契约。

P0-1 implementation complete / Human/provider deployment sign-off pending。

## 实施结果

- 新增 `app.user_roles` 和 Go 静态 capability policy：moderator、editor、admin 可叠加；普通用户没有角色记录。
- 新增标准 `flag` 实现的 `adminctl`、Windows 可用的 `pnpm adminctl:dev`。角色写操作只使用 owner/migrator；幂等授予、最后一个有效 admin 保护、角色事件与最后权限移除后的会话撤销均在事务内完成。
- Admin 只接受已验证本地邮箱、有效密码、active/未删除账号及当前角色；复用 easyhash Argon2id、dummy verification、CAS 升级及 User/credential 锁，不创建新会话表。
- Admin 登录、注销、再认证、`/me`、CSRF、会话列表和撤销接口均由 OpenAPI 定义，Go/TypeScript/sqlc 输出由生成器产生。
- Admin SPA 的 `/login` 与受保护 `/` 已实现角色展示、会话管理、再认证和注销；开发端口为 5173。
- 加入 Redis HMAC subject/global 限流和有界失败事件。密码重置/修改密码原子撤销所有 Public/Admin 会话，并只建立一个 Public 替代会话。
- 更新 CHANGELOG、Agent 路由文档、契约、开发与安全文档；没有新增依赖或未来产品 Domain。

## 安全边界

Admin 与 Public 使用不同 cookie 名称及数据库 session kind；即使将另一类 cookie 的值改名提交，也无法跨边界认证。
Admin cookie 为 HttpOnly、SameSite=Strict、Path=/、无 Domain，生产使用 Secure 的 `__Host-` 名称。
Admin 会话绝对期限 8 小时、空闲期限 1 小时、touch 间隔 5 分钟；每次请求读取当前角色。

不安全请求要求精确 `ADMIN_ORIGIN`；已认证写请求还要求独立 `ADMIN_CSRF_SECRET` 派生的 HMAC。
缺失、错误、重复请求头、Public CSRF、其他 Admin 会话 CSRF 及轮换前的 CSRF 均被拒绝。
前端只临时在内存中使用密码和 CSRF，原始会话 token 不进入 JavaScript。

角色操作先取得固定事务 advisory lock，再锁目标 User，避免并发撤销最后 admin。
移除最后 privileged role 会在同一事务撤销 Admin 会话；普通注销/撤销只作用于自身 kind。
密码 reset/change 是明确例外：撤销两类会话，只替换 Public。

限流键仅为 `gfp:auth:limit:` 加 HMAC 指纹，值只含有 TTL 的计数器。原始邮箱、密码、IP、OAuth subject 和 token 不进入键或值。
两维计数通过 EVAL 原子检查、递增并饱和；不使用 KEYS/SCAN，也不信任转发 IP 请求头。
Public login 10/subject + 500/global/15m，Admin login 5 + 100/15m；注册/reset 请求 3 + 200/1h；Public password verification 5/User + 500/global/15m，Admin reauth 5 + 100/15m。
成功清除 subject failure bucket，不清除 global。已获准的并发 KDF 可完成，但失败事件受饱和计数限制。

Redis 故障时 Public 本地认证 fail-open，并跳过无法保证有界的失败事件；Admin login/reauth fail-closed。
现有 PostgreSQL 会话继续工作。受限 reset 保持同一 202/body 且不发 challenge；OAuth 保持自己的 Redis 一次性 flow 依赖。

## 数据库与真实开发环境

`00005_admin_auth_roles.sql` 已先在全新 disposable PostgreSQL 验证，再通过准备好的 migrator 应用至 `gfp_dev`。
原迁移 1–4 未改写，迁移 5 现亦属于不可改写的已应用历史。River 继续使用官方 migration mechanism 和独立 `river` schema。

新增角色表先撤销 inherited default grants，再给予 Admin/readonly SELECT。API/Worker 无角色表权限，Admin 无角色 DML。
`gfp_admin` 得到 users/email identities/password credentials/roles/sessions 的必要 SELECT、sessions/security_events INSERT、session activity/revocation 列 UPDATE、User `updated_at` 行锁能力和 credential `password_hash/updated_at` CAS 升级能力。
它不能更新密码 epoch、账号状态、身份、角色、profile 或 challenge；没有 ownership/DDL/直接 sequence 权限。

真实 Admin smoke 使用 `gfp_admin`、`gfp_api`、`gfp_migrator` 的实际身份，验证临时已验证 moderator 账号与 HTTP 认证流程。
Admin smoke 的验证 token 只在进程内捕获；独立 Public auth smoke 验证真实私有文件邮件捕获。临时账号及关联记录已由 owner 精确清理。
全程未 SSH、修改共享服务器、cluster roles 或共享 Redis ACL。真实 `.local` 配置未打印、替换、删除或提交。

## 实际验证命令与结果

以下 Go 命令从 `server` 执行，其余 pnpm/Git 命令从仓库根目录执行。
集成测试只在 `CI=true`、`GFP_DISPOSABLE_INFRA=1`、`GFP_AUTH_INTEGRATION=1` 时启用，固定连接本机 `gfp_ci` 和 disposable Redis。
Windows race 检查使用 `CGO_ENABLED=1` 及现有 MinGW 工具链。

| 命令/验收 | 结果 |
| --- | --- |
| `pnpm check` | PASS；最终包含 8 项 Node 客户端测试、Go 单元测试、ESLint/gofmt/vet、TypeScript/Astro、三 Go runtime 和两前端构建 |
| `pnpm lint`、`pnpm typecheck` | PASS；Astro 20 文件 0 errors/warnings/hints |
| `go test ./...`、`go test -count=1 -timeout=3m ./...` | 最终 PASS；后者带 disposable guards，包含集成测试 |
| `pnpm integration:ci` | PASS；全新迁移 1→5、重复 up、官方 River、四身份 driver smoke、Public/OAuth/Admin/roles/throttle 回归 |
| `go test -count=1 -timeout=3m -run TestIntegrationAdmin ./internal/transport/public` | PASS |
| `go test -race -count=1 -timeout=5m -run TestIntegration ./internal/transport/public ./internal/redisstore ./internal/oauthprovider` | PASS；最终完整 race 回归三个包均通过 |
| `go test -race -count=1 -timeout=3m -run TestIntegrationAdmin ./internal/transport/public` | PASS；含 User 锁前的受控竞态与角色撤销期间请求 |
| `go test -count=1 -timeout=3m -run TestIntegrationAdminSessionsAndCSRF ./internal/transport/public` | PASS；含最终重复 CSRF 请求头修复 |
| `pnpm migrate:dev` | PASS；共享开发迁移 5 和官方 River 路径；仅 owned-object grants，无集群权限调整 |
| `pnpm smoke:admin:dev` | PASS；真实身份/授权、登录/me/CSRF/轮换/列表/撤销、Public/Admin 隔离、最后角色撤销与临时账号清理 |
| `pnpm smoke:dev` | PASS；实际 pgx/sqlc、Redis、River enqueue/execution/completion/cleanup/shutdown |
| `pnpm smoke:auth:dev` | PASS；最终使用真实 Redis throttle、真实 Public 认证/邮件捕获/恢复/会话/CSRF/清理 |
| `pnpm smoke:oauth:dev` | PASS；Google/GitHub 凭据对、固定 callback、S256 URL、Redis 一次性 flow；不包含真人授权交换 |
| `pnpm build:images` | PASS；Server/Web/Admin/PostgreSQL 四个本地镜像，无发布 |
| `docker build --file deploy/docker/server.Dockerfile --tag gofurry-server:p0-0-local .` | PASS；最后 CSRF 变更后的 Server 镜像复核 |
| `pnpm generate`、`pnpm check:generated` | PASS；再次生成后字节及文件集合均无 drift |
| `pnpm audit:repository` | PASS；Secret、Tailnet、namespace、dependency、architecture boundary 检查 |
| `git diff --check`、`git diff --cached --check` | 最终 PASS |
| `git diff --exit-code -- server/go.mod server/go.sum pnpm-lock.yaml server/db/migrations/00001_foundation.sql server/db/migrations/00002_identity_local_auth.sql server/db/migrations/00003_auth_security_recovery.sql server/db/migrations/00004_oauth_identity.sql` | PASS；依赖及既有迁移未改动 |
| `git check-ignore server/env/api.local server/env/admin.local server/env/worker.local server/env/migrator.local .local/readonly.env` | 五个真实配置均被忽略 |

补充实际操作：

- `node .cache/p01d-throttle-preflight.mjs`：临时、被忽略的 driver 预检，API/Admin 现有 Redis 凭据均通过 EVAL/SET/INCR/EXPIRE/GET/TTL/DEL；仅使用随机、带 TTL 的测试键。
- `go run ./cmd/adminctl grant-role -email p01d-ui-sept09@example.invalid -role moderator -database gfp_ci`、同地址 `list-roles -database gfp_ci`、`revoke-role -role moderator -database gfp_ci`：PASS；该地址只存在于 disposable 浏览器测试 fixture。
- `pnpm dev:admin` 配合在 disposable 环境启动的 `./bin/gofurry-api.exe`、`./bin/gofurry-admin.exe`：浏览器验证未登录跳转、登录、角色展示、再认证、注销、撤销角色后拒绝登录；全部 PASS。临时页面和进程已关闭。
- 本机验证使用已有专用 WSL Docker 环境；本任务只创建 `gfp-p01d-postgres`、`gfp-p01d-redis`，最终通过 `docker rm -f -v gfp-p01d-postgres gfp-p01d-redis` 清理。

开发中曾出现模块目录选择错误、编译适配错误和旧阶段的 Admin 404/schema v4 断言失败；均已修正并重跑通过，没有忽略失败或降低 Worker/API 权限断言。
新增规格文档仅将四处 Markdown 双空格硬换行转换为反斜杠硬换行，以通过 staged whitespace 检查，规格正文未改变。

## CI 与人工验收分离

开始时 `dev`/`origin/dev` 为 `7b3d6a07743503baf93efafb4c3cb87b4240b21e`，工作区只有用户提供的未跟踪 P0-1D 规格。
通过 `gh run list --repo deepfurry/gofurry-platform --branch dev --limit 5 --json databaseId,headSha,status,conclusion,url,workflowName` 确认
[P0-1C 远端 CI 34318443835](https://github.com/deepfurry/gofurry-platform/actions/runs/34318443835) 成功。
本轮 P0-1D 的 disposable CI 流程在本机完整执行通过；依照不 push 的要求，没有触发本提交的远端 Actions。

| 人工/部署门槛 | 状态 |
| --- | --- |
| Interactive Google OAuth login/callback/link/reauth/unlink | NOT RUN — deferred human acceptance |
| Interactive GitHub OAuth login/callback/link/reauth/unlink | NOT RUN — deferred human acceptance |
| 操作员使用真实账号完成 Admin 浏览器签字 | NOT RUN — deferred human acceptance；disposable 浏览器自动验证已通过 |
| 生产 Cloudflare Access 强制 MFA、生产邮件、OAuth 配置、私有 secrets、可信代理、Turnstile 决策及 secret management | 部署前由操作员完成 |

无剩余实施 blocker。未加入应用 TOTP/WebAuthn、Cloudflare Access JWT 校验、动态 RBAC、产品管理控制台、密码泄露 API 或未使用依赖。
本轮只在本地 `dev` 提交，提交说明为 `feat: add admin authentication and auth hardening`；最终 SHA 见本次任务回复或 `git log -1`。
`main`/`origin/main` 保持 `c2760399957548269c7fa8cb69bfbc45870c561d`，未 push、merge、创建 Git tag 或 release。
