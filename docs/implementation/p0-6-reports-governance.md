# P0-6 — 举报与治理（路线图阶段 1.3）

**状态：已实施并验收（2026-09-24）。** 实际结果见[验收记录](p0-6-verification.md)。
设计基线为 `dev` / `e73e756e9e7f77d35c6461aeba04ae472bb405a5`。
[设计前审计](p0-6-baseline-audit.md)确认阶段 1.1、1.2 已完成。本阶段对应总纲 P0-6，
不是继续扩展 P0-3 贡献类型；以一次包含前后端、数据与验收的完整交付收口阶段一。

## 目标与边界

打通「发现资源/来源问题 → 私密举报 → 后台核查 → 有权者处理 → 留痕 → 举报人查看安全结果」。
同时让业务限制立即生效、信任档位控制明确额度，并为阶段 2.1 提供可测试的推荐资格规则。

本阶段交付六项能力：资源/来源举报、范围限制、基础信任额度、治理操作与业务审计、
人工来源核查、收录/展示/推荐资格区分。保留已有七种贡献和普通 curation 流程。

不做用户举报、讨论/交换治理、自动封禁/下架、自动提升信任、爬虫/URL 探测、定时巡检、
邮件通知、申诉工单、上传、搜索/推荐页面、收藏或通用规则引擎。
不增加 Redis key、River job、ORM、JSONB/EAV、trigger、RLS、集群角色或基础设施组件。
“人工核查”是后台真实可操作的记录/处理流程，不以线下代办替代必要页面；服务端不访问提交的 URL。

### 本轮已确认的产品决定

**业务限制保留举报、登录、账号安全和本人记录。** 不通过 `account_state=disabled` 实现业务限流，
不撤销认证能力或修改产品角色。恶意举报使用独立额度，不增加 report_submit 封禁范围。
账号安全保留密码修改/恢复、邮箱验证、OAuth 管理、会话查看/撤销及退出；读取本人历史、
撤回本人待审贡献/未处理举报也保留。未来功能须明确映射范围，不能用“拦截所有非 GET”实现 all_write。

## Required Reading / Context Discipline

实施前先审计当前分支、工作区、history 和真实 schema，按顺序只加载相关上下文：

1. 本文、[基线审计](p0-6-baseline-audit.md)、[P0-3B 验收](p0-3b-verification.md)。
2. [AGENTS.md](../../AGENTS.md)、[架构路由](../../.agents/architecture.md)、[playbook](../../.agents/playbook.md)。
3. [架构](../../contracts/architecture.md)、[数据库](../../contracts/database.md)、[开发](../../contracts/development.md)、[生成](../../contracts/generated-code.md)契约。
4. [中文 roadmap](../product/roadmap.md)、[阶段一设计](../product/stage-1-contribution-governance.md)、
   [内容政策](../product/content-policy.md)、[信任与治理](../product/trust-safety.md)、[发现规则](../product/discovery.md)相关部分。
5. Auth capability/事务原语、identity profile 写入、contribution quota/投影、curation mutation/Tx 入口、
   Public SQL/SSR 缓存、OpenAPI、实际 Admin 导航与相关测试；不预加载整个 docs。

现有 spec/契约描述当前行为，本文明确列出的变更是本阶段的目标；实施时同步更新受影响契约。
出现权限、依赖环、秘密安全或共享 Infra 能力的实质冲突，报告证据，不能放宽权限绕过。

## 权限与角色

| 能力 | 普通用户 | Moderator | Editor | Admin |
| --- | --- | --- | --- | --- |
| 提交/查看本人举报 | 有效 Public session；提交要求邮箱已验证 | 同普通用户 | 同普通用户 | 同普通用户 |
| 举报队列、内部详情、受理/驳回/解决 | 无 | 普通问题可处理；敏感类别只受理/转交 | 无 | 全部 |
| 普通 canonical 编辑/贡献采纳 | 无 | 无 | 有 | 有 |
| 记录人工来源核查 | 无 | 可记录观察，不改 Resource | 可记录及普通 availability 修改 | 有 |
| rights、restricted/removed、软删除、推荐排除 | 无 | 无 | 无 | 有 |
| 用户限制、信任档位、全局审计 | 仅本人安全摘要 | 无 | 无 | 有 |

沿用 Moderation / Editorial / Administration，不增加动态 RBAC 或角色管理 HTTP API。
Moderator 无法修正资源时转交 Admin（其同时具备 Editorial），不增加一套 Editor 派单系统。
Editor 可以继续从既有编辑入口修正内容，但不因此看到举报人、举报正文或内部备注。
审核者不能决定本人提交的举报；Admin 不能给自己调整信任或施加/撤销业务限制。
工作人员的 Public 业务行为遵守同样限制，Admin 工作权限仍由 owner 管理的角色决定。

## 举报

### 提交、额度与隐私

目标仅为当前公开 Resource 或其当前公开 Source。使用 Resource ID、可选 Source ID 和明确 target_kind；
必须验证 Source 归属。不存在与不可见统一安全 404；canonical localization 损坏仍返回 500。
资源页面仅添加普通链接进入 private React 举报表单，匿名 Astro 页面仍不 hydration。

原因是闭合枚举：broken_link、rights_concern、malicious_link、privacy、content_rating、spam、other。
正文为 1～4,000 Unicode 字符的普通文本；不上传附件、不渲染 HTML、不自动抓取证据 URL。
原始目标、原因和正文不可编辑；撤回后可重新提交。URL 参数只携带类型/目标定位，不能携带正文或凭据。

所有信任档位统一使用独立举报额度：滚动 24 小时 10 份、未终结 5 份、间隔 60 秒。
同作者、同目标、同原因只能有一份未终结举报；其他举报人的数量/存在性不泄漏。
request_id + 规范化摘要支持同请求回放；同键不同内容 409。回放在额度/目标复查前，仍须验证当前 session/所有权。
撤回/驳回不退还当日额度；撤回释放未终结名额。User 锁序列化额度和重复举报，不能只依赖按钮禁用。

作者 DTO 仅包含本人原稿、状态、安全消息与当前仍公开的目标链接；Source URL/Resource 标题不复制成“用户原稿”。
目标后来隐藏时，自己的正文和状态仍可读，canonical 名称/URL/关联对象、内部备注、staff ID、
其他举报、限制调查材料和审核版本均不进入作者 DTO。本人可粘贴的原始文字不等于服务器可以补充隐藏信息。
私有 API/pages 全部 no-store，页面 noindex；沿用现有 session、精确 Origin、CSRF 和严格 decoder。

### 处理状态

```text
open → in_review → resolved / dismissed
open / in_review → withdrawn（本人）
```

终态不可重新打开；重复请求按请求键回放或返回状态冲突。处理动作使用 expected_report_version，
每次实际状态/转交变更 bump 一次；stale 不覆盖。只有一个终态成功。
copyright/隐私/恶意链接对应的 rights_concern、privacy、malicious_link 默认高优先级并送 Administration；
Moderator 可补充内部核查或转交，不能以“驳回”代替无权作出的敏感裁定。
其余为正常优先级，按优先级、创建时间、ID 排队；不以举报数量自动下架或加权推荐。

受理、转交、核查、解决、驳回和撤回均写不可变事件；安全消息与内部备注用不同字段。
解决/驳回要求给作者的说明（1～1,000 字符）；内部备注最多 2,000 字符，不默认复制到作者消息。
重复问题可以在内部关联另一条举报，但作者不获得那条举报的 ID 或内容。

“解决”有两种明确依据：

- 已执行操作：在同一事务执行一个受控治理动作并结案，或关联同目标的既有业务审计/治理动作。
- 无需改动：明确记录已恢复、无法复现等结论及说明；不得虚构已下架/已修复。

需要资源修改的请求必须携带当前 Resource expected_version。不能先调用会自行 commit 的 curation 方法，
再在第二个事务标记“执行成功”。复杂普通编辑可先在既有编辑页完成，随后显式关联那条实际审计；
这两步是清楚分开的操作，不伪装为一次原子按钮。
首版复合结案只接受四种闭合操作：Resource publication、Source availability、Source rights、
推荐排除；分别按既有能力检查，不支持任意字段更新、关系编辑或账号处罚嵌套。
triage 仅受理、转交 Administration 或追加内部核查说明；追加说明写 noted 事件并推进 report version，
不能借此编辑原稿或作者可见结果。撤回在作者 User/Report 锁内判断非终态，无需向作者暴露内部处理版本。
Resource 没有创作者/所有者归属模型，禁止从某条贡献或举报自动推断“资源责任人”并惩罚账号。

## 业务限制与信任额度

### 三个当前范围

| scope | 阻止的行为 | 明确保留 |
| --- | --- | --- |
| contribution_submit | 七种新贡献提交 | 本人记录、撤回、举报、账号安全 |
| public_profile_write | 公开 handle/display_name/bio 修改及开启索引 | 关闭搜索引擎索引、本人读取、其他未限制业务 |
| all_write | 上述两类业务写入；后续业务须逐一接入 | 举报、账号安全、本人记录/撤回、关闭索引 |

如果一个请求既包含获准的隐私设置又包含被限制字段，整份拒绝，不部分写入。
限制不会自动隐藏当前公开资料、改变 Resource、拒绝已有贡献或使账号无法登录。
已有待审贡献仍可由有权者审阅/采纳；确需拒绝时显式处理，不根据作者限制批量改历史。

Admin 选择范围、理由代码、用户可见说明、内部备注和有效期；首版期限为 24 小时/7 天/30 天/长期，
撤销必须说明理由。记录创建与撤销时间/操作者；已过期记录不改写成“被撤销”。
同范围不重复叠加有效限制；新限制不能暗中修改旧记录的起止时间。到期由数据库当前时间判断，
无需 worker/Redis/cron；重设或延长期限通过显式替换，在同一事务撤销旧记录并创建新记录，
保留两条历史，不能因延长限制出现短暂放行窗口。创建请求可带 replaces_restriction_id，仍须匹配同用户/范围并检查 CAS。

新建/撤销限制、调整信任与业务写入均锁同一目标 User。涉及操作者和目标两名 User 时先按 UUID
顺序锁两者，再复核操作者 session/capability；禁止“先锁 actor 再任意锁 target”造成相互操作死锁。
User 治理配置带独立 revision/CAS；没有配置行解释为 New、revision=0，首次写入在 User 锁内创建为 1。
到期只改变派生有效状态，不制造后台版本更新。判定时刻取锁后数据库时间，不使用事务开始前的旧时间。

### 固定信任档位

| 内部档位 | 贡献 / 滚动 24 小时 | 最大待审 | 最短间隔 |
| --- | --- | --- | --- |
| New（所有现有账号默认） | 10 | 5 | 60 秒 |
| Established | 30 | 10 | 30 秒 |
| Trusted | 60 | 20 | 15 秒 |

这些是首版固定策略参数，不是评分算法或流量容量承诺。由 Admin 以原因和审计手工调整，
不自动晋级、不按每个用户任意配置额度、不赋予后台角色、不免审核、不增加举报额度。
降级只限制后续提交，不删除超额的已有待审记录。限制优先于新增额度，回放已有成功请求仍允许返回原 ID。
提交与本人额度展示共用策略和查询，不能只改 checkQuota 而仍显示旧的 10/5/60。

本人“账号状态”只显示有效限制、用户说明、到期信息和实际可用额度；不暴露内部信任等级/分数、
内部备注、操作者或调查依据。Public error 区分业务限制 403 与额度 429，提供安全原因/适用 Retry-After。

## 内容资格与治理时效

保留三层不同判断，不引入单一“安全/合法/可信”布尔值：

| 判断 | 本阶段规则 |
| --- | --- |
| 可收录 | 编辑审核确认与平台相关；历史/不完整资源可以保留，沿用现有 canonical 字段和人工判断 |
| 可公开展示 | 现有 published/未删除/分类未删除；Source 非 removed 且 rights 为 unknown/creator_provided/confirmed；关系对端也公开 |
| 可主动推荐 | 先满足公开条件，再要求 general、active、分类 active、未被 Admin 排除，并至少有一个 active 且 rights=creator_provided/confirmed 的来源 |

unknown rights 的来源可以展示，但不满足本阶段主动推荐的来源条件；source_type 不是版权或安全保证。
仅为推荐添加每 Resource 的 normal/excluded 策略，缺行等于 normal。普通浏览/精确查询不因 excluded 消失；
本阶段不做排名、搜索页、推荐页或索引。提供纯资格函数和 PostgreSQL 候选过滤的一致性测试，供 2.1 接入。
推荐排除的创建/解除由 Admin 操作，使用 Resource CAS、bump 与审计；不暴露内部排除理由。

版权核查沿用 rights_review/disputed/removed_by_request；URL 不再列出，元数据可继续公开。
availability=restricted 在现有规则中仍公开，不能把它当隐藏按钮；Editor 也不能通过修改 availability
绕过 rights 封锁。Resource restricted/removed、软删除仍为 Administration，无新增父对象 restore。

**明确变更 P0-2B 的缓存合约：** 本阶段将四个匿名 Public Resource/Taxonomy read endpoints 及
Resource SSR 页面改为 no-store，删除 s-maxage/stale-while-revalidate；纯 SSR、无凭据转发和 SEO 规则保留。
理由是首版没有缓存 purge 基础设施，需要让治理提交后的新请求按最新公开资格响应。
已加载页面不会被远程擦除；不能保证提交前已在途的快照消失。部署已有缓存时还需等待旧缓存到期/清除，
不能宣称仅改响应头就能追回旧内容；当前本地设计不触发 CDN 配置或部署。以后恢复共享缓存另定失效方案。

## 来源健康

后台提供按异常 availability、未处理 broken_link 举报和最近核查时间查看来源的入口。
人工核查记录 outcome=reachable/unreachable/uncertain、观察时间、说明、操作人和检查时的
Resource revision/规范化 URL 指纹；不持久化网页响应、下载文件或抓取凭据。
URL 后来修改时，旧核查只保留为历史，不能作为新 URL 的当前健康结论。
仅记录观察也须校验提交的 Resource expected_version，并在锁内绑定当前 Source/URL 指纹；
stale 要求重新核对，不能把针对旧地址的观察静默记到新地址。说明仅记录来源观察，不复制举报人或举报正文。

Moderator 可以追加观察记录，不能因此修改 canonical availability；Editor/Admin 可显式选择
“记录并更新”普通 availability，仍走 curation、Resource CAS 和同一事务审计。
reachable 不自动恢复已移除来源、不解除 rights；uncertain 不自动设置失效；移除需要单独明确选择。
记录观察本身不 bump Resource；availability 实际改变才 bump。重复 request_id 不重复记录，
新的核查即使结论相同也可记一次观察，不能冒充新的 canonical 变更。
Public 首版沿用现有 availability 展示，不新增“安全下载/保证合法/验证安全”徽章。

## 应用层、审计与事务

只增加实际用到的两个边界：

- `internal/governance`：固定信任/额度/限制策略、事务内有效限制查询、闭合类型的审计写入原语。
  可以使用 pgx/sqlc，但不得依赖 Auth、Identity、Curation、Contribution 或 transport。
- `internal/moderation`：举报与治理 Application，编排 Auth、governance、curation 的窄 Tx 入口；
  也编排受限制的 profile 写用例，调用 identity 字段策略/事务内更新。不要反向让 identity 导入 auth。

Contribution 调用 governance 的政策检查；Curation 调用其审计原语；两者不导入 moderation。
Auth 继续只负责认证与 live capability，Resource/Taxonomy 继续纯领域。不要建立通用 repository、
回调命令执行器、工作流引擎或让 browser 选择任意 SQL/动作函数。
普通 profile handler 转到受保护的用例，不能保留另一个绕过检查的运行入口。

一般 mutation：锁 User → 复核 session/capability/有效业务限制 → 锁举报或治理配置并校验 CAS →
锁 canonical parent → 执行变更 → 写事件/审计 → 提交。关系仍是 graph lock → UUID 顺序两端，
不把报告锁或任意 Resource 锁插到图锁之后形成反向路径。用户间治理先按上文锁所有 User。
复合 Resource 治理只处理一条报告/一个主资源，不接受任意批量混合变更。
独立 curation 审计不反向锁 Report；结案引用已存在的 audit_id。复合操作关联 report_id 时必须
先锁 Report 再锁 Resource，连同外键可能取得的锁一并核对，禁止 Resource→Report 的反向路径。

独立的 curation 方法保留自己的事务；供复合操作复用的 Tx 方法不 commit，并仍验证能力。
审计失败、角色/会话撤销、CAS stale 或资格变化均完整 rollback；no-op 不 bump、不伪造 canonical 审计。
私有多查询读取必须保持一致投影和当前授权，沿用 1.2 已验证的语义。

从本阶段生效时起，所有正式 Resource/Taxonomy mutation（直接编辑、贡献接受、治理执行）
记录 actor、动作、对象、时间、受影响字段集合及版本；关系记录双端。高影响治理另有理由、
报告关联和明确的前后 publication/rights/distribution/trust/限制状态。
不为旧数据补造审计，不把 Auth security_events 或 Contribution 原稿复制成通用日志。
大段正文/URL 不复制到审计，可记录闭合字段名、变化计数/摘要指纹；需要原稿时走既有授权历史。
不用任意 metadata map、JSONB 或通用 field/value 明细表保存差异。
全局审计仅 Administration；Moderator 读自己的授权举报处理记录，Editor 不因能编辑而获得举报私密信息。

## 数据模型与迁移

预计只追加 `00009_governance_foundation.sql`，Goose 8→9；已应用 00001～00008 内容/hash 不变。
不修改十张 Resource Core 表及其 grants，不扩大身份/角色/凭据权限，不改 gfp_* / gfp: Infra contract。
建议八张有明确用途的关系表，最终 DDL 按 sqlc/约束落地，不能增建未来领域：

| 表 | 关键内容与约束 |
| --- | --- |
| reports | reporter、闭合 Resource/Source 目标、原因/原稿、request_id/hash、状态/version、处理队列；FK/归属检查；原稿列不可 UPDATE |
| report_events | 不可变 submitted/triaged/escalated/noted/resolved/dismissed/withdrawn 事件，safe_message/internal_note 分列，终态唯一 |
| user_governance_profiles | User PK、New/Established/Trusted、revision、更新时间；无行默认 New/0 |
| user_restrictions | User、scope、用户说明/内部原因、起止和撤销记录；生效按数据库时间，User 锁防同范围并发重叠 |
| resource_distribution_policies | Resource PK、normal/excluded；理由放治理动作；变化参加 Resource CAS |
| source_checks | Source/Resource、URL 指纹、观察结论/时间、检查基准、staff 和说明；追加式，不改变旧核查 |
| moderation_actions | 闭合治理动作、操作者、类型化目标、reason、可选 report、request_id/hash；不可变，不存任意 payload |
| audit_entries | 类型化对象/动作/字段集合、前后版本及必要状态、关联操作/贡献；关系逐端记录；无敏感正文/凭据 |

目标使用具体 FK 列和 CHECK 约束，不用无法校验的万能 target_id 代替归属；历史中被硬删除的
关系/绑定可以保留明确的历史 ID，不能因此阻止正常删除或声称仍有 FK。所有时间为 timestamptz。
幂等键至少在 actor/operation 范围唯一；分页/队列、本人历史、有效限制、Source 最近核查设必要索引。
不得使用依赖 now() 的部分唯一索引判断“当前有效限制”；用 User 锁、时间条件和普通索引解决。

| 角色 | 新权限 |
| --- | --- |
| gfp_api | 举报输入列 INSERT、本人读取所需列 SELECT、撤回状态列 UPDATE；公开事件列 INSERT/SELECT；信任/限制的判定与本人安全投影列 SELECT；推荐策略 SELECT。无内部备注/审计/治理写入权限 |
| gfp_admin | 新表必要 SELECT/INSERT、报告状态/版本/队列、信任与撤销字段、推荐策略等明确列 UPDATE；不可改原稿/事件/动作/审计历史，不得新获 role/account_state/credential DML |
| gfp_worker | 无新对象或 Resource Core 权限 |
| gfp_readonly | 新表 SELECT，无写权限 |

先清理新对象继承的默认授权再明确 GRANT；应用层按 reporter/能力过滤，不能依赖前端过滤或 RLS。
Admin 用户治理按精确 User UUID 定位（举报/贡献提供已授权 ID），不新增邮箱枚举、模糊用户搜索或后台分配角色。
Down 9 仅 guarded disposable gfp_ci；新治理数据存在时拒绝，不能静默删记录。
测试清理自己创建的新夹具后 9→8→9，证明七种旧贡献、Resource 和权限保持；共享 dev 只 up。

## API 与页面

OpenAPI 先定义再生成；下列是本阶段的用途边界，字段按上文闭合语义落实，不能用自由 action map：

| Public API | 用途 |
| --- | --- |
| POST /reports | 创建举报，request_id 幂等 |
| GET /me/reports、GET /me/reports/{id} | 本人列表、原稿及安全处理结果 |
| POST /me/reports/{id}/withdraw | 本人撤回未终结举报 |
| GET /me/governance | 当前有效限制、用户说明和实际业务额度；不返回内部 trust |

| Admin API | 用途与权限 |
| --- | --- |
| GET /reports、GET /reports/{id} | Moderation 队列/详情；状态、原因、处理组、分页筛选 |
| POST /reports/{id}/triage、/resolve、/dismiss | 受理/转交/解决/驳回，report CAS；复合 resolve 使用闭合可选变更类型及 Resource CAS |
| GET /users/{id}/governance；PUT /users/{id}/trust | Administration 精确用户读取/调整信任，治理配置 CAS |
| POST /users/{id}/restrictions；POST /users/{id}/restrictions/{restriction_id}/revoke | Administration 创建/撤销，原因/幂等/CAS |
| PUT /resources/{id}/distribution | Administration 设置/解除推荐排除，Resource CAS |
| GET /source-checks；POST /resources/{id}/sources/{source_id}/checks | 核查列表与记录；可选 availability 修改额外要求 Editorial |
| GET /audit、GET /audit/{id} | Administration 分页/对象/动作筛选与必要差异 |

普通 curation endpoints 保留；高影响 mutation 请求增加必填治理理由，并同步生成客户端和 UI，
不留可绕过审计/理由的新旧双入口。没有新 HTTP 角色管理、account disable 或 Auth decoder 扩容。
Resource description 仍 50,000 Unicode chars；Auth 8 KiB、其他 Admin ceiling 256 KiB 保持。

Public 页面：`/report`、`/me/reports`、`/reports/[id]`；账号页增加业务状态入口。
Admin 页面：`/reports`、`/reports/$reportId`、`/governance/users/$userId`、`/sources/health`、`/audit`；
推荐排除和理由输入整合既有 Resource editor。`/` 仍转 `/resources`。
角色导航准确区分资源只读与举报可处理，不能给 Moderator 整个工作区都标成不可操作。
使用普通 textarea、显式预览/确认、dirty guard、错误/空状态；成功 refetch、冲突保留输入、不自动 retry、
无 optimistic mutation 或 autosave。举报处理明确预览“举报人将看到的消息”，防止误发内部备注。

## 连续实施与验收

内部按三个工作段连续推进：①迁移/政策/事务入口与审计；②举报、治理和来源核查前后端；
③跨流程与回归验收。它们不是三个独立立项，不按每张表/每个按钮再拆任务，也不交付占位 UI。

| 验收面 | 必须证明 |
| --- | --- |
| 举报与隐私 | 公开目标/Source 归属、不可见 404、损坏 500、原稿不可改、本人/他人/内部 DTO 隔离、隐藏后的安全历史、独立额度/回放/重复目标 |
| 状态与权限 | Moderator/Editor/Admin、敏感类别转交、自审禁止、终态竞争、report CAS、角色撤销、无权复合动作完整失败 |
| 限制与身份 | 建立/撤销/到期与贡献/profile 并发；双 User 锁序；静态审计 P2-01 回归；all_write 下举报/认证安全/本人读取/撤回/关闭索引仍可用 |
| 信任额度 | 三档、降级、缺行 New、有效限制优先、跨七种贡献共享额度、本人额度一致；没有免审核或后台权限提升 |
| 治理与审计 | direct curation/贡献接受/复合治理都原子留痕；审计故障回滚；双端关系版本；no-op 不伪造变更；原因必填且无旧入口旁路 |
| 资格与时效 | 展示和推荐不同；版权隐藏 URL 仍可保留元数据；availability restricted 不能冒充隐藏；no-store、SEO/无岛/无凭据转发回归 |
| 来源健康 | Moderator 只记观察，Editor 普通修改，rights 仅 Admin；URL 改变使旧核查不再当前；重复键、相同结论、stale/回滚；零远程抓取 |
| 数据与升级 | migration 1～8 hash、Resource/身份 grants 保持；新表逐列授权；Down 拒绝有治理数据；9→8→9 兼容七种旧贡献 |
| 浏览器闭环 | 举报 → 受理/转交 → 修正或治理 → 结案 → 本人结果；限制/解除 → 表单实际受控；核查 → 来源展示变化；全部既有流程回归 |

实施阶段实际运行：

```text
pnpm generate
pnpm check
pnpm integration:ci
pnpm build:images
pnpm migrate:dev
pnpm smoke:admin:dev
pnpm generate
pnpm check:generated
git diff --check
```

继续扩展既有 Admin smoke：临时举报人/审核者/目标账号和 Resource graph，验证举报、限制、
信任、来源核查及 Admin→Public 治理闭环；只清理本次夹具，不触碰真实 Human Acceptance 账号。
CI 固定 disposable PostgreSQL/Redis，不读 *.local；共享 Goose 目标 9，只用准备好的 migrator up。
不 push、不 merge main、不 tag/release 或生产部署。更新 CHANGELOG/契约/中文 roadmap 和实际验收记录，
审查完整/staged diff 后本地 dev 提交，建议 `feat: add reports and governance`。

只有上述功能与验收完成才关闭 1.3 / 阶段一，随后进入 **2.1 搜索与发现**。
本阶段已完成；“完整 P0 完成后再正式上线”的约定保持不变。
