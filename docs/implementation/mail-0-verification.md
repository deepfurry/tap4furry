# MAIL-0 verification

日期：2026-09-19。基线：`a13175bab218115f27c33dcb547f9589b38ec718`；分支：`dev`。
实施依据为同目录 `mail-0-production-transactional-mail.md`，仅规范化附件中的 Markdown 行尾空格。

## 实现与边界

- 新增 `internal/mail.Resend`，使用官方 `github.com/resend/resend-go/v3` v3.17.0
  的 `Emails.SendWithContext`，通过私有窄接口进行离线测试。该版本源码确认不自动重试。
  Go module 仅新增此依赖及两条 checksum，没有升级其他依赖。
- 复用原有 `ChallengeMailer`。Auth 生产代码仅将提交后投递预算从 2 秒改为 5 秒；
  不改变 challenge、事务、注册失败处理、验证重发及重置枚举防护语义。
- 验证和重置邮件包含纯文本、最小 HTML、过期时间及忽略提示；链接仍为 fragment token。
  没有远程图片、脚本、tracking、CC/BCC、附件或 provider metadata。
- 仅 Public API 组合 Local / Resend / Disabled。development/test 保留 local 默认值；
  production 必须显式选择 resend，并提供非空私密 key 和有效的 From/Reply-To 地址。
  Resend 不要求本地 capture 目录；Admin/Worker/Migrator 不读取 mail 配置。
- 固定官方 HTTPS endpoint，禁止重定向，HTTP 与 context 均有 5 秒上限。
  provider/network 错误不包装、不输出，统一为静态投递失败。
- 扩展审计：识别 Resend/generic API key、smoke 收件人和人工验收凭据；拒绝追踪私密路径，
  并检查 Resend import 仅在 mail boundary。审计只输出安全结论或文件路径。
- 新增显式 `pnpm smoke:mail:resend:dev`；只读取 ignored 私密输入，生成内存随机合成 token，
  发送一次验证样式邮件。不依赖 Auth/database/Redis/Jobs，不创建 User/challenge，拒绝 CI。
- 无 SMTP、queue、retry、outbox、webhook 或模板系统；无新增/修改 migration、API 或前端。
  未修改 shared Infra、DNS、Cloudflare、Resend domain、`gfp_*` 或 `gfp:` contract。
  production raw token 不进入持久化存储、队列、日志；开发私密 local capture 能力保留。

## 已执行验证

| 命令/检查 | 结果 |
| --- | --- |
| 基线 `pnpm check` | PASS |
| `go -C server get github.com/resend/resend-go/v3@v3.17.0`、`go -C server mod tidy` | PASS；仅预期 SDK 依赖变更 |
| `go -C server test -count=1 ./internal/mail ./internal/config ./internal/auth` | PASS |
| `node --test scripts/tests/private-values.test.mjs scripts/tests/mail-smoke.test.mjs` | PASS；3 项新增脚本测试 |
| `pnpm exec eslint .` | PASS |
| 最终 `pnpm check` | PASS；审计、生成漂移、lint/vet、类型、11 项 Node 测试、Go 单元测试、Web/Admin/三个 runtime 构建 |
| 随后的 `pnpm generate`、`pnpm check:generated` | PASS；生成文件字节与文件集合无漂移 |
| `CI=true GFP_DISPOSABLE_INFRA=1 pnpm integration:ci` | PASS；全新 disposable PG18/Redis8、Goose/River 两次 up、四角色 driver smoke、P0-1 集成/并发/隐私回归 |
| `pnpm build:images` | PASS；Server/Web/Admin/PostgreSQL 四个镜像；image inspect 确认命令及 runtime 用户 |
| `pnpm audit:repository` | PASS；真实私密值仅在内存比较，未输出 |
| 私密输入/migration 完整性检查 | PASS；7 个已有私密输入和 migrations 00001～00005 字节一致，没有新增 migration |
| `go -C server list -deps ./cmd/mail-smoke` | PASS；无 Auth/database/Redis/Jobs 依赖 |
| `git diff --check`、`git diff --cached --check` | PASS；附件四处行尾空格已规范化 |

普通 Go 测试仍按原有 guard 跳过 disposable integration；表中的独立 integration 命令
实际启用了这些用例，包含 mail-after-commit、注册投递失败仍保留 session、重发失败映射、
已知/未知/不符合资格/投递失败重置请求的一致响应。普通 mail 单元测试覆盖 local capture
和 Disabled 回归。CI 明确覆盖为 disabled mode 并清空 Resend smoke key/recipient；
Resend 单元测试仅用 fake sender/transport，不进行外部邮件网络调用。

本机 Docker Desktop 启动遇到 `Wsl/Service/WSL_E_CONSOLE`，验收复用此前已有的专用本地
WSL Docker 环境。最初两次 image build 分别因 WSL relay 地址限制和旧 build proxy 端口失败；
修正仅限 ignored 本地验收辅助配置，没有修改 tracked Dockerfile 或 shared Infra。

## 真实 Resend smoke

无凭据、无邮件投递的 provider 连通性预检得到预期 HTTP 401。
实际 `pnpm smoke:mail:resend:dev`：PASS。使用已经存在的 ignored 输入，未修改文件；
只调用一次真实 Resend adapter，Resend 接受一封 synthetic verification email。
无 User/challenge/数据库变更，token 随进程退出释放，不可通过应用兑换。
PASS 只表示 provider 接受请求；没有声称已验证最终收件箱或用户点击。
报告不包含 key、收件人、token、验证/重置 URL、provider response body 或 message ID。

## Production contract

```dotenv
MAIL_MODE=resend
MAIL_FROM=Tap4Furry <no-reply@tap4furry.com>
MAIL_REPLY_TO=support@tap4furry.com
```

`RESEND_API_KEY` 通过私密运行环境注入，绝不作为 image build 输入。
附件第 21 节示例 Reply-To 末尾多余的 `>` 不沿用；默认值遵循第 7/14 节的正式 sender contract。
Open/click tracking 继续由已有外部配置保持关闭，本次没有查询或修改该配置。
生产部署、最终收件箱送达确认及部署安全配置属于独立验收。

## 最终状态

本次 disposable PostgreSQL/Redis 容器已删除，专用验收 daemon 与临时 relay 已停止。
共享开发账户、凭据、数据库和 Redis 未被本阶段修改。

MAIL-0 验收通过，可以进入 P0-2 — Taxonomy & Resource Core。
本地提交消息：`feat: add production transactional email`；最终 SHA 由提交后报告给出。
不 push、不 merge main、不 tag/release。
