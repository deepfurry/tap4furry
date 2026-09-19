# BRAND-0 实施与验收报告

日期：2026-09-19。基线为 `dev` 的
`6fa46f693b8232b0bb4fc497328c33f98c44e729`，对应
[CI 35345781264](https://github.com/deepfurry/tap4furry/actions/runs/35345781264)
已通过。开始时唯一未跟踪文件是用户提供的 BRAND-0 规格，与下载文件内容一致。
规格仅将 Markdown 双空格硬换行改为反斜杠硬换行，以通过 whitespace 检查。

## 迁移结果

- 产品、Web/Admin 页面标题与文案、私有开发邮件主题和 OpenAPI 标题统一为 Tap4Furry。
- Go module 与全部自有 import 为 `github.com/deepfurry/tap4furry/server`。
- 根包为 `tap4furry`；workspace 包及消费入口统一为 `@tap4furry/*`，exports 保持不变。
- Public/Admin 生产 Cookie 分别为 `__Host-tap4furry_session`、
  `__Host-tap4furry_admin_session`；本地去掉 `__Host-`。
- OAuth 绑定 Cookie 为 `tap4furry_oauth_google`、`tap4furry_oauth_github`，
  生产添加 `__Host-`。保留 Secure/HttpOnly/Path/Domain/SameSite、十分钟 TTL 和清除语义。
- 开发默认 CSRF/Admin CSRF/throttle 字符串、三个 runtime binary 和四个镜像改名；
  二进制为 `tap4furry-api`、`tap4furry-admin`、`tap4furry-worker`，镜像为
  `tap4furry-{server,web,admin,postgres}:local`。
- 未来生产 origin/callback 文档统一为 `tap4furry.com` 与 `admin.tap4furry.com`；
  本地 4321/5173 origin、8080/8081 API 和 callback 推导逻辑未变。
- CSS 自有变量前缀迁为 `--tap4furry-`，取值、布局和视觉样式未变。
- 更新 README、Agent 指导、contracts、产品/架构/工程文档和已完成阶段的现行契约。
  历史执行命令保持真实，并注明其历史性质。CHANGELOG 添加 BRAND-0 Unreleased 项。

## 保留项与边界

`gfp_dev`、`gfp_ci`、`gfp_api`、`gfp_admin`、`gfp_worker`、`gfp_migrator`、
`gfp_readonly`、`gfp_runtime` 和 Redis `gfp:` 保持不变。
它们是稳定基础设施标识，不是产品品牌。Goose 00001～00005 未修改，无新增迁移。
`app`/`river` schema、权限边界、CI fixture 与一次性基础设施保护均未变。

外部 easyhash 模块、版本、校验和及 API 用法保持原样；所有依赖版本未升级。
pnpm 正常刷新只改变 workspace 名称/排序，并补充三条现有平台包的 libc 元数据。
Go 无需执行 `go mod tidy`，`go.sum` 完全未变。

五个既有私有配置文件执行前后的 SHA-256 相同，未输出内容或摘要。
Secret/Tailnet 审计通过。没有共享数据库迁移、集群角色/Redis ACL 修改、SSH、部署、
OAuth 凭据修改或生产配置变更。smoke 只使用既有配置及自身临时 fixture，并清理自身数据。
Cookie 改名后需重新登录；没有兼容别名，没有批量撤销、清空或迁移既有会话。

## 实际验证

| 命令 / 操作 | 结果 |
| --- | --- |
| `git branch --show-current`、`git status --short`、`git log -5 --oneline`、`git remote -v` | PASS；dev、指定基线、已迁移的 canonical origin |
| `gh run list --repo deepfurry/tap4furry --branch dev --limit 5 --json databaseId,headSha,status,conclusion,url,workflowName` | PASS；基线 CI success |
| 修改前 `pnpm check` | PASS；建立当前实现基线 |
| `pnpm install --lockfile-only --offline` | PASS；按 workspace 名称刷新，依赖版本不变 |
| `pnpm install --frozen-lockfile` | PASS；未批准或新增依赖构建脚本 |
| `pnpm generate` | PASS；只由 oapi-codegen/sqlc/Orval 生成，Go/sqlc 输出未变、TS 标题更新 |
| 修改后 `pnpm check` | PASS；审计、生成漂移、ESLint/gofmt/vet、类型、8 项客户端测试、Go 测试、Web/Admin/三个 binary 构建 |
| `CI=true GFP_DISPOSABLE_INFRA=1 pnpm integration:ci` | PASS；全新本机 PG18/Redis8、Goose/River 两次 up、四角色 driver smoke、Public/Redis/OAuth 集成与并发测试 |
| `pnpm smoke:dev` | PASS；真实 pgx/sqlc/Redis/River enqueue、执行、完成、清理和 shutdown |
| `pnpm smoke:auth:dev` | PASS；真实认证/邮件捕获/验证/恢复/轮换/CSRF/profile/logout 与临时账户清理 |
| `pnpm smoke:oauth:dev` | PASS；Google/GitHub 凭据对、固定 callback、S256 与一次性 Redis flow；所有值隐去 |
| `pnpm smoke:admin:dev` | PASS；角色权限、Admin 登录/me/CSRF/轮换/会话隔离、撤销和临时账户清理 |
| `pnpm build:images` | PASS；Server/Web/Admin/PostgreSQL 四个新名称镜像；image inspect 与容器内二进制列表确认新 command/name |
| 再次 `pnpm generate`、`pnpm check:generated` | PASS；生成文件字节和文件集合一致，无 drift |
| `go -C server list -m github.com/deepfurry/tap4furry/server github.com/gofurry/easyhash` | PASS；新自有 module 与外部 easyhash v1.2.0 正常解析 |
| `git diff --exit-code 6fa46f693b8232b0bb4fc497328c33f98c44e729 -- server/db/migrations` | PASS；覆盖全部五个迁移，零差异 |
| `node .cache/brand0-verify.mjs` | PASS；私有文件/迁移字节一致、Infra 标识不变、Go/pnpm 依赖版本校验和不变、规格正文一致、分支边界正确；临时脚本未提交 |
| `rg -n --hidden -g '!.git' -i 'gofurry\|go-furry\|gofurry-platform\|gofurry\.com\|@gofurry\|__Host-gofurry' .`（表格转义的竖线按正则或执行） | 见下方逐项分类；零未解释残留 |
| 最终 `pnpm audit:repository`、`git diff --check`、`git diff --cached --check` | PASS；包含完整最终 diff 与暂存区审阅 |

命令中的 CI 变量在 PowerShell 以 `$env:CI='true'`、`$env:GFP_DISPOSABLE_INFRA='1'`
设置，未传入真实开发地址。四个本地镜像使用已有专用 WSL Docker 环境构建，没有发布。
本轮一次性容器 `gfp-brand0-postgres`、`gfp-brand0-redis` 已通过
`docker rm -f -v gfp-brand0-postgres gfp-brand0-redis` 清理；daemon 和临时代理已停止。
普通 Go 测试按原有 guard 跳过集成用例；上表的 integration 命令实际启用了这些用例。
没有新建或弱化认证逻辑，现有测试补充了 Public/OAuth Cookie 的完整新名称断言。

## 人工后续与下一阶段

- GitHub repository Homepage 若仍为旧域名，由操作员改为 `https://tap4furry.com`。
- Google/GitHub OAuth 应用显示名可改为 `Tap4Furry Dev`；无需重新创建 client、改 Secret
  或改现有 localhost callback。本任务未进入提供方控制台修改设置。
- 真人 Google/GitHub consent、登录/绑定/再认证/解绑和操作员 Admin 签字本轮未执行，
  属于 P0-1 后续签字项，不是 BRAND-0 阻塞项。
- P0-1 Identity/Auth implementation complete；P0-2 Taxonomy & Resource Core next。

仅在本地 `dev` 提交；无 push、main 合并、Git tag/release 或部署。
提交 SHA 由最终交付消息提供，也可用本文件最后一次提交查询。

## 残留旧品牌逐项审计

扫描覆盖 Git 跟踪文件及本次新增文档，包括隐藏的 Agent/CI 配置；不读取被忽略的私有文件。
规格中的原名、迁移映射、扫描样例和禁止项必须保留，不能将规格本身改成已完成状态。
下表将每条命中按文件及行号列出；不存在未解释的当前产品品牌残留。

<!-- residual-inventory -->

共 114 条命中行，全部分类。以下行号对应本次最终文件内容。

| 文件 | 命中行号 | 保留原因 |
| --- | --- | --- |
| `docs/engineering/tech-stack.md` | 54 | 外部 easyhash 依赖/原始 API 或验收引用，必须保留 |
| 用户提供的 BRAND-0 实施规格 | 1, 17, 36, 275 | 规格标题、改名前基线或独立项目说明 |
| 用户提供的 BRAND-0 实施规格 | 151, 152, 277, 286, 292, 293, 294, 295, 296, 407, 700, 889, 1042, 1222, 1236, 1237, 1238, 1239, 1240, 1241, 1242, 1243, 1244, 1245, 1303, 1501 | 规格原始扫描规则/禁止项，保留规则语义 |
| 用户提供的 BRAND-0 实施规格 | 263, 347, 1230, 1361, 1414, 1493 | 外部 easyhash 依赖/原始 API 或验收引用，必须保留 |
| 用户提供的 BRAND-0 实施规格 | 316, 328, 380, 392, 393, 394, 395, 412, 442, 443, 444, 479, 480, 481, 482, 514, 515, 516, 517, 554, 588, 612, 624, 661, 673, 731, 732, 733, 808, 809, 898, 938, 939, 940, 941, 942, 943 | 规格中的旧 → 新映射输入，保留迁移依据 |
| 用户提供的 BRAND-0 实施规格 | 794, 1081, 1102, 1106, 1449, 1453, 1457 | 规格记录提供方/仓库元数据旧值，供操作员后续核对 |
| `docs/implementation/brand-0-verification.md` | 61 | 外部 easyhash 依赖/原始 API 或验收引用，必须保留 |
| `docs/implementation/brand-0-verification.md` | 64 | 本报告记录实际残留扫描正则，非产品名称 |
| `docs/implementation/p0-0-repository-bootstrap.md` | 85 | 既有共享开发主机的禁止 SSH 条款，非产品名称；不改 Infra |
| `docs/implementation/p0-0-repository-bootstrap.md` | 239, 1288 | 外部 easyhash 依赖/原始 API 或验收引用，必须保留 |
| `docs/implementation/p0-0-validation.md` | 103 | 已标注的历史执行命令/验证环境名，保留实际证据 |
| `docs/implementation/p0-1a-identity-local-auth.md` | 119, 229, 675, 1405 | 外部 easyhash 依赖/原始 API 或验收引用，必须保留 |
| `docs/implementation/p0-1a-validation.md` | 52, 53, 56 | 外部 easyhash 依赖/原始 API 或验收引用，必须保留 |
| `docs/implementation/p0-1b-session-security-verification-recovery.md` | 157 | 外部 easyhash 依赖/原始 API 或验收引用，必须保留 |
| `docs/implementation/p0-1b-validation.md` | 54 | 外部 easyhash 依赖/原始 API 或验收引用，必须保留 |
| `docs/implementation/p0-1b-validation.md` | 55, 71, 72 | 已标注的历史执行命令/验证环境名，保留实际证据 |
| `docs/implementation/p0-1d-verification.md` | 78, 89, 98 | 已标注的历史执行命令/验证环境名，保留实际证据 |
| `README.md` | 47 | 用户提供的实施规格文件路径引用，保持可定位 |
| `server/go.mod` | 9 | 外部 easyhash 依赖/原始 API 或验收引用，必须保留 |
| `server/go.sum` | 72, 73 | 外部 easyhash 依赖/原始 API 或验收引用，必须保留 |
| `server/internal/auth/challenge.go` | 13 | 外部 easyhash 依赖/原始 API 或验收引用，必须保留 |
| `server/internal/auth/password_test.go` | 4 | 外部 easyhash 依赖/原始 API 或验收引用，必须保留 |
| `server/internal/auth/password.go` | 10 | 外部 easyhash 依赖/原始 API 或验收引用，必须保留 |
| `server/internal/mail/local_test.go` | 12 | 外部 easyhash 依赖/原始 API 或验收引用，必须保留 |
| `server/internal/transport/public/admin_integration_test.go` | 29 | 外部 easyhash 依赖/原始 API 或验收引用，必须保留 |
| `server/internal/transport/public/integration_test.go` | 29 | 外部 easyhash 依赖/原始 API 或验收引用，必须保留 |
| `server/internal/transport/public/recovery_integration_test.go` | 15 | 外部 easyhash 依赖/原始 API 或验收引用，必须保留 |
| `server/internal/transport/public/security_integration_test.go` | 18 | 外部 easyhash 依赖/原始 API 或验收引用，必须保留 |
