# 阶段 1.3 设计前审计

日期：2026-09-23。基线：`dev` / `e73e756e9e7f77d35c6461aeba04ae472bb405a5`。
开始时工作区干净，领先本地记录的 origin/dev 7 个提交；本轮未 fetch 或 push。

## 结论与范围

阶段 1.1、1.2 的贡献能力已有实际代码、生成合约和验收记录，可以继续设计 1.3。
本次针对下一阶段依赖的事务、权限、私有投影、数据库授权和页面入口审计，未发现需要
推翻现有架构的冲突，也未发现本审计范围内有证据支持的 P0/P1 问题。
这不是全仓库安全认证，不把尚未实现的治理能力误报为已交付功能缺陷。

已核对用户提供总纲的 P0-6 范围，以及仓库路线图、阶段一设计、内容政策、信任与治理文档、
相关工程契约和实现。上一阶段完整 CI、真实 smoke、镜像与浏览器证据见
[P0-3B 验收记录](p0-3b-verification.md)；本轮没有重跑那些数据库写入/容器验收，也没有据此声称新增治理功能已验证。

## 需要在 1.3 补齐的接入风险

### P2-01：资料写入缺少事务内会话与业务限制接入点

证据：`server/internal/transport/public/auth.go` 的 `UpdateProfile` 将已解析 actor 的
User ID 传给 `server/internal/identity/identity.go:96`；后者开启事务并直接更新 profile。
`server/db/queries/identity.sql` 只检查账号 active/未删除，不检查 initiating session，也不锁 User。
因此在请求完成鉴权后、资料写事务开始前撤销会话，单凭该写入路径不会再次发现撤销。

这是静态代码确认的并发授权缺口；本轮未对真实账号进行故障复现。当前影响限于在途资料写入，
不是任意用户越权。若把新业务限制仅接在 HTTP 层，同样会产生“限制已提交，旧请求仍写入”的问题。

1.3 实施时先补齐：应用层在 User 锁内复核当前 session 和有效业务限制，再在同一事务写入 profile。
Auth 已依赖 identity，不能反向引入 `identity → auth` 环；保留 identity 的字段策略，增加窄的事务内写入口，
由具备 Auth/治理上下文的应用用例编排。贡献提交沿用现成 User 锁，不另建认证系统。

验收：用可观察的数据库锁等待覆盖会话撤销、限制建立/撤销/过期与 profile 写入的竞争；
认证与安全恢复接口仍可用。该项作为 1.3 实施前段的必要工作，本次设计提交不声称已经修复。

## 已确认的基础与设计差异

| 实际证据 | 结论 / 1.3 处理 |
| --- | --- |
| `auth/roles.go` 已定义 Moderation、Editorial、Administration；AdminShell 目前只显示资源/贡献/分类/账号 | 复用静态 capability，新增治理入口；Moderator 能处理举报但仍不能写 canonical Resource；全局 “Read-only” 提示需限定到资源编辑场景 |
| `contribution/submit.go`、`read.go` 固定 10/24h、5 pending、60 秒；DB 无 Trust/Restriction 表 | 增加固定信任档位和 PostgreSQL 有效限制查询，提交与本人额度展示使用同一策略函数 |
| 贡献接受与 curation 共享事务，关系共享图锁并审计双端 | 保留；不重建贡献审核系统，不因治理引入通用命令总线 |
| `change_read.go` + `snapshot` 做一致投影与当前授权复核 | 举报/治理私有读取复用同样的安全语义，不拼接跨时点公开资格 |
| `curation` 常规 mutation 自行提交，现有 audit 只覆盖贡献采纳 | 普通正式知识变更补原子业务审计；“执行治理并解决举报”必须调用受控 Tx 入口，不能先修改后另记成功 |
| `public_resource.sql` 展示 active/unavailable/broken/restricted，rights 仅允许 unknown/creator_provided/confirmed | availability=restricted 并不隐藏 URL；版权处理使用现有 rights_review/disputed/removed_by_request，普通失效移除使用 removed |
| Go Public 与 Astro 均使用 s-maxage=60、stale-while-revalidate=30 | 这是既有 P0-2B 合约，不是本轮新漏洞；1.3 若承诺治理后新请求立即隐藏，需要调整缓存语义，不可只检查数据库 |
| 尚无举报、信任、范围限制、来源核查或推荐资格表/API | 追加迁移与最小权限；不把设计状态标为功能完成，不增加后续 Search/Collection 领域 |

现有代码中的“无恢复”指 Resource/Category/Tag 软删除无 restore。
Source 行保留，availability 可以通过现有编辑变更；真正版权封锁必须由 Administration 控制的
rights 状态承担，不能依赖 Editor 也能改的 availability 阻止重新展示。

## 本轮实际检查

- 分支、status、最近 history、跟踪迁移清单：PASS，基线符合上一阶段提交，恰有迁移 1～8。
- 只读共享开发审计 `node .cache/p03b-grants.mjs`：PASS，Goose=8；gfp_api/admin/worker/readonly
  的 Resource Core 对象/列权限与已有 ACL 指纹完全一致。辅助脚本不提交，未输出私密值。
- `node .cache/p03b-preserve.mjs`：PASS，迁移 1～7 和已有私密输入与前一阶段记录逐字节一致；
  本轮不改任何迁移，包括已应用的 8。
- 代码与测试覆盖核对：已检查双端 CAS/图锁、审计失败回滚、标签退休竞争、作者投影及快照撤权测试。
- `pnpm check`：PASS；`pnpm generate` 后 `pnpm check:generated`：PASS，生成文件逐字节一致，无新增/删除。
- `pnpm audit:repository`：PASS，Secret、命名空间、依赖与边界审计通过，未输出私密值。
- 本轮五份文档的 43 个本地链接检查：PASS；迁移 1～8 与 HEAD 逐字节比较：PASS，未创建 migration 9。
- `git diff --check`：PASS。最终改动仅为审计、设计、路线图和 CHANGELOG；没有把阅读测试代码
  等同于重新运行数据库集成测试，也没有声称新增治理能力已经实现或通过验收。

## 后续范围

依上述审计形成 [P0-6 / 阶段 1.3 设计](p0-6-reports-governance.md)。
用户已确认：业务限制必须保留举报、登录、账号安全与本人记录，恶意举报通过独立额度防护。
本轮只修改设计和路线图，不运行迁移、不创建治理表、不修改现有产品权限或私密配置。
