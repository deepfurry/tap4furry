# P0-3B — 完整贡献类型（路线图阶段 1.2）

**状态：已实施并完成验收。** 本文为阶段 1.2 的实施规格，实际命令、浏览器流程、并发/隐私与迁移验证见[验收记录](p0-3b-verification.md)。
审计基线：`dev` / `5f3aafa303f75a4c89d7c09070df9f1c4f54ec0c`（`feat: add basic contribution review`）。
阶段 1.1 已完成；本阶段作为一次包含前后端与验收的完整交付，不按贡献类型再次拆成独立项目。

## 目标与范围

用户不仅能提交资源和纠错，还能补充来源、标签、关系及翻译，并在同一个贡献中心跟踪结果。
审核者继续使用已有队列，查看原稿、基准、当前内容及最终修改，修订后接受或拒绝。
与 1.1 合并后，总纲 P0-3 的七种贡献全部可用。

| 类型 | 本阶段交付 | 接受后的结果 |
| --- | --- | --- |
| 新增来源 `add_source` | 为公开资源补充一个外部 URL、来源类型和说明 | 新增 Source，版权状态为 unknown |
| 移除失效来源 `remove_broken_source` | 选择当前公开来源并说明失效依据 | availability=removed；保留 Source 和历史 |
| 添加标签 `add_tag` | 为一个资源选择 1～10 个现有有效标签 | 仅增加选中的绑定，不替换整个标签集合 |
| 添加关系 `add_relation` | 选择另一个公开资源、关系类型和方向 | 按现有图规则建立关系，两端各增加一个版本 |
| 添加/修订翻译 `add_translation` | 为一个非默认语言新增或修改名称、摘要和描述 | 写入该语言的 Resource localization |

沿用 `create_resource`、`update_resource` 的现有行为。每份提议只有一种类型、一个主资源；
标签允许有限多选，关系只有一条，翻译只有一种语言。不是任意多对象混合变更包。

不增加标签/关系移除提议、taxonomy 创建或翻译、来源恢复、版权裁定、Resource 删除/恢复、
外部 ID 提议或默认语言切换。这些不属于总纲此次新增类型；已有后台管理能力继续可用。
举报、来源健康记录、信任与限制属于 1.3；搜索、收藏、认领、通知、上传、自动翻译和远程 URL 抓取不进入本阶段。

## Required Reading / Context Discipline

实施开始先核对分支、状态、历史及当前实现，再按顺序阅读：

1. 本文，以及 [1.1 规格](p0-3a-basic-contribution-review.md)与[实际验收记录](p0-3a-verification.md)。
2. [AGENTS.md](../../AGENTS.md)、[架构路由](../../.agents/architecture.md)、[执行流程](../../.agents/playbook.md)。
3. [架构契约](../../contracts/architecture.md)、[数据库契约](../../contracts/database.md)、[开发契约](../../contracts/development.md)、[生成契约](../../contracts/generated-code.md)。
4. [阶段一设计](../product/stage-1-contribution-governance.md)、[领域模型](../product/domain-model.md)、[内容政策](../product/content-policy.md)、[隐私与治理](../product/trust-safety.md)的相关部分。
5. `internal/contribution`、`internal/curation`、Public/Admin OpenAPI、相关 SQL 和现有贡献页面/测试；按实际修改读取详细文件，不一次加载整个 `docs/`。

产品路线以[中文 roadmap](../product/roadmap.md)为准。总纲提供范围依据，仓库契约与实际代码决定复用边界；
出现实质冲突先汇报，不能通过改写历史迁移、扩大 Resource 权限或另建审核系统绕过。

## 实际基线与必要扩展

| 审计结果 | 1.2 的处理 |
| --- | --- |
| migration 7 只允许 create/update 两种提议；内容表要求基础资料完整 | 追加 migration 8，扩展封闭类型约束并新增类型化记录；不为新类型伪造基础资料快照 |
| `ApplyReviewedTx` 只支持创建/基础纠错；常规 curation 方法会自行提交 | 增加受控类型分派和事务内复用点；仍由 contribution 统一提交，不嵌套调用已提交的方法 |
| 既有关系写入统一图锁、顺序锁两端，但只对入口资源比较 expected_version | 贡献接受额外校验两端提交版本；复用相同图锁/环检查，不改变既有 Admin API 的请求契约 |
| 现有提议结果与审核记录只记录主资源版本 | 关系提议保存对端基准，追加每个受影响资源的版本审计；不能只审计一端 |
| Public 普通读取对翻译做字段回退 | 私有翻译上下文读取目标语言原始行，区分不存在、NULL 和回退展示 |
| 作者历史已区分实际输入和服务器补齐字段 | 新类型沿用该边界，并对来源、标签及关系对端独立检查当前公开资格 |

这是在既有应用层上的必要扩展，不重构 Auth、Resource/Taxonomy domain 或数据库基础设施。

## 共同产品规则

- 沿用待审核、已接受、已拒绝、已撤回；原稿不可编辑，修改通过撤回/重新提交并关联原提议。
- 提交要求有效 Public session 和已验证邮箱；本人读取/撤回保持 1.1 的权限。Editor/Admin 审核，Moderator 不获得贡献审核权，不允许自审。
- 所有新类型只针对当前公开资源。关系的两端必须都公开。取得上下文、提交和接受都检查相关资格，隐藏与不存在对作者使用相同的安全响应。
- 理由沿用 1～2,000 Unicode 字符；审核修订必须说明原因。安全消息与内部备注分开，作者不能读取内部备注。
- 所有类型共用滚动 24 小时 10 份、待审 5 份、提交间隔 60 秒的现有额度；标签多选作为一份。接受不新增提交次数；撤回/拒绝不返还当日额度。
- 沿用 request_id、规范化请求摘要和作者锁。相同请求回放在额度检查之前；同 ID 不同内容冲突。标签顺序不影响摘要；修改内容后生成新 request_id。
- 不自动接受、合并、重试或重定基准。过期提议保持待审，可拒绝或由作者撤回后基于当前内容重新提交。
- 接受必须产生真实知识变更。重复 Source、已存在关系、已绑定标签、相同翻译等不能制造空的 accepted 记录或增加版本；整份无效时不作部分接受。

### 审核可以修改什么

沿用已确认的“允许修订后接受，完整留痕”。类型、主资源、被移除的 Source、关系两端与有向顺序、翻译目标语言不可替换。

| 类型 | 审核可修订内容 |
| --- | --- |
| 新增来源 | URL、标签文字、来源类型；确认可用状态 |
| 移除失效来源 | 处理说明；接受的动作固定为移除，不改成修复/替换 URL |
| 添加标签 | 调整待添加的标签集合，最终仍为 1～10 个有效、尚未绑定的标签 |
| 添加关系 | 保持原方向，在三种有向类型之间修正；最终类型重新执行去重及环检查 |
| 翻译 | 同一语言内修正文本；不改默认语言或夹带基础资料修改 |

原稿、最终采用内容和修订说明都保留。修改不能解除旧版本冲突；部分标签不合适时可明确修订列表后整份接受，不能静默跳过失败项。
`related_to` 与有向关系不在审核时互换，避免把对称关系的 UUID 存储顺序误当作作者表达的方向；需要改变这类语义时重新提议。

## 各类型的行为

### 来源

新增来源输入 URL（最多 2,048 字符）、可选 label（最多 80）、source_type（现有枚举，默认 unknown），以及贡献理由。
使用现有纯函数规范化 HTTP(S) URL，不访问目标网站；去重范围仍为 `(resource_id, normalized_url)`。
即使重复项已移除或因版权隐藏，也不能新建重复项、自动恢复旧项或向作者返回隐藏项详情。
作者只收到“无法添加此来源”等安全错误；有权审核者可在后台查看原因。

作者不能设置 rights_status、is_primary 或 availability。审核者显式确认 active/unavailable/broken 之一；
接受后 rights_status 固定 unknown、is_primary=false，不自动覆盖现有主来源。
没有主来源也允许新增非主来源；需要调整主来源时使用现有 curation，不把主来源调整混入提议。

移除必须指定该资源当前可公开读取的 Source ID，并提供失效依据；不要求它预先被标为 broken。
接受时再次校验归属和公开资格，只改 availability=removed。保留 URL、版权状态及原有 primary 标记，
不自动选替代来源；现有模型允许零个公开主来源。以后恢复/调整由独立 curation 操作处理。
标为 broken/unavailable 仍会出现在现有 Public 列表，因此不能以这些状态冒充“移除”。
版权争议不是普通失效移除的裁决入口，交由具备治理权限的既有后台或后续 1.3 处理。

### 标签

从现有公开有效标签选择 1～10 个；规范化为去重、固定排序的 UUID 集合，不接受自由文本创建标签。
只添加这些绑定，保留其他标签和既有退休绑定，不调用“全量替换标签”来覆盖无关内容。
提交/接受都检查未删除、active、尚未绑定；接受时锁相关 Tag，阻止与退休/删除并发穿透。
taxonomy 更新不增加 Resource.version，因此不能只依赖 Resource CAS 代替标签资格检查。
任何最终选中项失效则整份失败，保持待审；审核者可明确修订列表后再次处理。

### 关系

通过公开资源的精确 slug 定位对端并确认名称，不增加 fuzzy search、搜索索引或 Admin 查询旁路。
支持 `part_of`、`successor_of`、`derived_from`、`related_to`。界面给出完整句子预览，例如“A 属于 B”，
可在提交前选择有向关系的方向；`related_to` 不需要方向并沿用 UUID 规范顺序。禁止自关联。

提交基准同时绑定两端 Resource.version。接受时任何一端变化或不再公开都失败，即使涉及的字段看似无关。
按最终关系类型执行重复/有向环检查；不同有向类型分别检查，不能把跨类型路径当成同一种环。
并发接受或直接 Admin 编辑共用同一个固定 advisory xact lock，不能另设“贡献图锁”。
成功时两端各 bump 一次，记录两端 before/after version；终态、历史和审计失败必须连同两端变更一起回滚。

### 翻译

这里只翻译 Resource，不翻译 taxonomy 或应用 UI。语言使用现有 BCP 47 规范化，区别于资源原始语言和 UI 语言。
目标必须是非默认语言；默认语言纠错继续走 1.1。不能删除翻译行或切换 default_locale。

新增语言要求 name（1～160 字符），summary 可选（最多 500），description 可选（最多 50,000 Unicode 字符）。
修订已有语言允许部分字段：省略表示保留，summary/description 显式 null 表示清空并恢复字段回退，name 不可清空。
界面用明确的“修改/保持/清空”状态区分这些操作；预览只作文本展示，不执行 Markdown HTML。

上下文返回目标语言行是否存在、该行真实可空字段，以及单独标注的默认语言参考；不能把回退值当成已有翻译存回。
没有翻译行时不能预填默认名称后静默当作作者译文。原稿只包含实际填写的字段，基准/补齐内容留在审核侧。
接受后仍按 requested→default 的现有逐字段规则读取，缺失 canonical 默认行返回 500 INTERNAL_ERROR。

## 上下文、版本与隐私

扩展现有 `/contributions/context/{slug}`，缺省仍提供 1.1 基础纠错上下文；新类型只返回各自需要的公开资料。
按类型接受闭合参数，如 source_id、目标 locale、关系对端 slug/方向/类型。无关或矛盾参数拒绝，不提供任意字段查询。

沿用不透明 HMAC 基准，增加用途区分并绑定作者、类型、主资源/版本/默认语言，以及该类型的目标标识；
关系再绑定对端/版本和端点顺序，翻译绑定规范语言，移除绑定 Source ID。不能跨作者、类型或目标复用上下文。
采用有边界的稳定编码，复用现有密钥边界；不暴露裸 Resource.version，不记录原始 HMAC，不新增私有配置。
上下文展示值与生成基准所读版本必须来自一致的读取快照；提交重读资格并核对基准。
Public 继续只读 Resource Core，不为取得基准申请 UPDATE/行锁权限。

所有 private API/page 使用 no-store，页面另设 noindex，沿用 browser session/CSRF；匿名 Resource SSR 的缓存、无 island 和不转发凭据规则保持不变。
作者原稿可保留本人确实提交的 URL、文字及选择 ID，但不能把服务器查出的名称/URL/语言回退内容伪装成用户原稿。
列表摘要同样遵守此规则；没有作者自填标题时用类型名称，不从隐藏资源补标题。

作者的安全结果按以下规则投影，不能直接序列化内部快照：

- 当前资源链接只在现在公开时返回。新类型的 accepted 内容至少要求当前 Resource 版本等于接受版本。
- Source 还要满足当前公开来源资格；移除成功只展示动作/安全消息，不回传已移除的 canonical URL 或内部状态。
- Tag 还要满足当前绑定公开资格；删除的标签不通过贡献历史恢复名称。已绑定退休标签沿用公开读取规则。
- Relation 的 accepted 内容要求两端同时公开、版本都未变化；任一端不满足时整体隐藏该关系结果，不能带出对端名称或链接。
- 翻译的默认语言参考只在有资格的编辑上下文中返回；未提交的基准字段不能进入作者历史的原稿。accepted 翻译按上述当前公开版本条件投影。
- 内部备注、版权状态、publication_state、deleted_at、裸版本和审核 actor 标识都不进入作者 DTO。

来源/关系/标签的资格检查与结果投影需来自一致快照，不能分散查询后拼出不同时间点的公开状态。
任一资源后来隐藏时，作者仍能查看自己的原稿、提议状态和安全消息；后台保留完整审核记录。

## 应用层与事务

`internal/contribution` 负责提议类型、权限、额度、状态、历史与审计；`internal/curation` 负责 canonical 变更。
扩展 `ApplyReviewedTx` 的类型化协作入口或同边界内的小型专用函数；不引入通用 command bus、插件审核器或 repository 抽象。
Resource/Taxonomy domain 仍不依赖 pgx、sqlc、Fiber 或 OpenAPI。

接受事务顺序：

1. 锁审核者 User，事务内重验 Admin session、当前 Editorial，锁提议并拒绝自审/已终结状态。
2. 关系先取现有固定图锁，再按 UUID 顺序锁两端；其他类型锁主资源。检查所有提交版本及当前公开资格。
3. 根据类型锁必要 taxonomy、检查来源/语言/关系不变量，在同一事务内调用 canonical 写入能力。
4. 有效变更每个受影响资源 bump 一次；写 accepted 类型快照、终态事件和完整版本审计后一起提交。

复用 READ COMMITTED、事务时间戳和现有 actor 锁序。底层 curation 入口也重验能力，不能接受调用者伪造“已经授权”。
贡献接受路径绝不能先走通用单资源锁再进入关系图锁，否则可能与已有关系写入形成反向锁序。
相关公开资格依赖的 Category/Tag 应沿用现有锁序重验；跨两端 taxonomy 锁使用稳定顺序。
角色撤销、重复接受、撤回竞争、版本冲突或审计失败均只允许完整成功或完整回滚。

## 数据模型与迁移

只追加 `00008_complete_contribution_types.sql`，预期 Goose 由 7 升至 8。
`00001`～`00007` 内容/hash 不变；不修改 Resource Core 表、grants、集群角色或 Redis ACL。
migration 8 扩展 contributions 的 kind/target CHECK，新类型都要求主资源和正数 base_version；
保留原有 create/update 数据及字段含义，submitted_fields 的旧位值不重解释，新类型不用它伪装基础资料字段。

实际新增以下五张有明确用途的类型表，DDL 与 sqlc/约束均已落实，不使用 JSONB/EAV：

| 表 | 保存内容 |
| --- | --- |
| `contribution_source_changes` | 新增/移除来源的 base/proposed/accepted；目标 Source、URL/label/type、该动作需要的可用状态等固定字段 |
| `contribution_tag_changes` | 各快照的选中 Tag 行及绑定基准；组合键避免同一快照重复标签 |
| `contribution_relation_changes` | 两端、方向/类型及快照，对端提交版本与 accepted 两端结果版本；主端提交版本继续由 contributions 持有 |
| `contribution_localization_changes` | 目标语言、base 行是否存在、文本与固定字段存在标记；区分无行、NULL 和未提交 |
| `contribution_review_resource_changes` | 关联既有审核记录，逐 Resource 保存 before/after version；关系有两行 |

base/proposed/accepted 都不可 UPDATE/DELETE。移除来源的历史不依赖 Source 被物理删除；FK 继续 RESTRICT。
标签的空 base 不插入行，读取时解释为选中集合尚未绑定；proposed/accepted 始终要求非空。
每种 kind 只允许对应快照组合；原始/接受内容必须完整且不可串用，跨表不变量由事务和集成测试保证，不新增 trigger/RLS。
新类型不要求插入 contribution_contents；更新目前内连接该表的列表/详情查询，避免新类型在队列中消失。
旧审核审计保留主端信息；新增逐资源审计用于新类型，读取兼容旧记录，不伪造历史或为旧记录补造审核事件。

| 数据库角色 | 允许变化 |
| --- | --- |
| gfp_api | 新类型表按所需列 SELECT/INSERT；仍不能读审核审计或改 canonical Resource；既有终态写权限不扩大 |
| gfp_admin | 新类型快照 SELECT/INSERT、逐资源审计 SELECT/INSERT；沿用既有贡献决策及 canonical DML |
| gfp_worker | 不获得上述对象或 Resource Core 权限 |
| gfp_readonly | 新表 SELECT，无写权限 |

先清除新对象继承的默认授权，再明确授权；runtime 无历史表 UPDATE/DELETE。
实施时把列级 grants 写入迁移和逐列验证，不依赖默认权限或授予 ALL。
已在排队的 1.1 提议必须能在升级后继续读取、接受、拒绝或撤回；旧已处理记录仍可读。

Down 仅供 guarded disposable `gfp_ci`。8→7 必须在有新类型数据时明确拒绝，不能静默删除真实贡献来恢复旧 CHECK。
测试先验证此保护，再由测试所有者仅清理自己的 1.2 夹具，执行 8→7→8 并证明预置 1.1 记录仍在；
原有 7→6→5→7 回归继续在隔离夹具中覆盖，最终恢复到 8。共享 gfp_dev 仅 migrate up。

## API 与页面

保留现有 5 个 Public、4 个 Admin 贡献 endpoint，扩展 context、提交、详情、类型筛选及接受请求的类型化契约。
OpenAPI 先于实现；使用闭合 kind + 对应 payload，未知字段、多个类型混装和非法组合必须拒绝。
延续两种旧类型请求/响应兼容性；选择具体生成表示时核对 oapi-codegen/Orval 产物，不用任意 map/object 逃避校验。
作者 DTO、审核 DTO 分开；纯 domain 不使用生成类型；不增设另一套 endpoint 字符串或未经生成的前端客户端。
沿用现有错误模型：未登录 401、无权限 403、不可见目标 404、基准/状态冲突 409、额度 429、canonical 损坏 500；
关系成环、重复/无变化等复用 curation 错误语义，UI 区分提示但不显示数据库细节。错误字段在 OpenAPI 中明确。
256 KiB body 上限、50,000 Unicode 描述限制与 Auth 8 KiB decoder 保持不变。

公共端继续使用 `/submit`、`/me/contributions`、`/contributions/[id]`，后台继续 `/contributions` 和详情。
资源页的“贡献/完善资料”普通链接进入提交页；类型选择发生在 private 页面，不给匿名资源页增加 React hydration。
提交页按类型切换精简表单；来源移除选择来源，标签多选，关系显示句子预览，翻译显示原始行与独立参考。
浏览器 URL 只传类型/目标定位，不带理由、正文或基准 HMAC。表单不持久化到 localStorage/sessionStorage。

后台一个队列支持七种类型筛选，详情按类型显示基准/原稿/当前/最终值。默认先显示原稿，明确修订模式和修订原因。
显式预览、提交、接受/拒绝；不自动保存。冲突保留表单，只能由用户明确重载或重新提议；不自动覆盖输入。
成功后 refetch canonical snapshot 与提议详情，不做乐观拼接；沿用 dirty route/beforeunload guard。
Markdown 只用 textarea 与转义文本对比，不引入第二个 HTML sink、富文本框架或客户端渲染器。

## 实施节奏与验收

本阶段内部按“类型模型/合约 → 事务与前后端闭环 → 集中验收”连续推进。
五种新提议共用基础流程，不分成五次独立审批或验收阶段；与 1.1 保持回归，完成后进入 1.3。

| 验收面 | 必须证明 |
| --- | --- |
| 类型完整性 | 五种新类型分别完成页面提交、审核修订、接受/拒绝/撤回及本人历史；旧两类不退化 |
| 来源 | URL 规范化/隐藏重复项安全错误、rights unknown、primary 不被偷改、移除保留行、移除后的历史不泄漏 URL |
| 标签 | 1～10 个去重、多选一次 bump、只增不删、退休/删除及其并发拒绝、单项失败整体回滚 |
| 关系 | 方向和 symmetric 规范化、自关联/重复拒绝、双端 stale、逐类型环、并发 cycle race、与直接 curation 的锁兼容、双端审计 |
| 翻译 | 新语言/部分修订/显式清空、无行与 NULL、不复制回退值、默认语言禁止、Unicode 上限、canonical corruption 500 |
| 共同事务 | 会话/角色撤销竞争、自审、接受/拒绝/撤回竞争、跨类型共享额度、请求回放、审计故障注入、no-op 不 bump |
| 隐私 | 自有/他人 ID、隐藏主端或对端、来源版权改变、标签删除、后续版本变动；列表、详情、accepted 与上下文均无旁路泄漏 |
| 前端 | 五种真实表单/差异/结果、冲突不覆盖、dirty guard、private no-store/noindex、无新增 HTML sink、匿名 SSR 无岛和无凭据转发 |
| 升级与权限 | migration 1～7 hash、Resource grants 不变、各角色对象/列权限、新旧提议共存、disposable Down 保护与 round-trip |

实施完成实际运行并记录结果：

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

扩展现有 `smoke:admin:dev`，使用临时作者/审核者和临时 Resource graph 完成五种新流程及公开结果验证，
只清理 smoke 自己创建的夹具；不新建每类贡献的 smoke 命令。CI 只用 disposable PostgreSQL/Redis，绝不读取真实配置。
再确认 shared Goose=8、无额外迁移、无 generated drift、迁移 1～7 和 Resource grants 不变、无 secret/真实地址被跟踪。
本地凭据不打印、不修改；共享 Infra 不执行 Down、不 SSH 或提权。

最终更新 CHANGELOG、契约及中文 roadmap，记录实际命令、浏览器验收、迁移/权限、隐私/并发结果和剩余问题；
审查完整与 staged diff 后在 dev 本地提交。建议实现 commit：`feat: complete resource contribution types`。
不 push、不 merge main、不 tag/release 或部署。只有实现和验收完成才把 1.2 标为已完成；本设计稿不算功能交付。

实现说明：私有投影采用一致快照，返回前再以 READ COMMITTED 复核授权，避免快照早于持锁的
角色/会话撤销；mutation 仍沿用单事务实时授权。OpenAPI 使用明确的类型字段，生成请求经过
严格 decoder 与应用层校验，强制一种 kind/一种 payload；请求拒绝响应专有字段和空标签集。
