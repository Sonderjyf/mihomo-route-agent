# 架构：本地控制器与可选云端决策

生产候选是一个 Go Windows exe：DNS Gate、规则/缓存、SQLite、回环 HTTP provider、Controller client、CLI。Jev 是可关闭的 HTTPS 后端；无 Python/CUDA 常驻。模型不直接修改配置。

`0.1.0-dev` 已实现 Gate/规则/有界内存/双 provider/CLI；SQLite、真实网络证据和外部恢复待开发。样例默认 off。[实测](PROTOTYPE_REPORT.md) 与 [运行说明](BUILDING.md) 区分当前能力和下文的产品目标。

## 模式

| 模式 | UNKNOWN 无缓存时 | 首次连接承诺 |
|---|---|---|
| async，建议正式版默认 | 立即明确 fallback，后台有限请求；采纳后提交 | 不保证首次连接走新规则 |
| bounded-preflight，实验 opt-in | 等 API 与 provider 提交；过期立即 fallback | 仅期限内成功且 hostname 能关联时覆盖 |
| 模型关闭 | 静态/手工/有效缓存 + fallback | 不做新 AI 决策 |

async 的独立后台任务有有限生命周期；写入前重新核对规则/策略版本。同步请求过期/取消后的迟到结果丢弃，不偷偷转后台补写。任何模式都不迁移已建立连接。

## 模块与复用

~~~text
cmd/route-agent/main.go      生命周期与 CLI
internal/route/domain.go    netip、IDNA、PSL、特殊域
internal/route/rules.go     单规则快照、优先级、Mihomo 规则生成
internal/route/agent.go     有界内存、三态、singleflight、期限
internal/route/gate.go      DNS UDP/TCP、上游、挂起与回答
internal/route/providers.go Controller、snapshot、提交与回读
internal/route/judge.go     Jev HTTPS client 与后端接口
internal/route/config.go    配置校验与 render
integration/flclash.js       幂等 profile 覆写（尚未实现）
~~~

当前复用 miekg/dns、x/sync/singleflight、x/net/idna/publicsuffix、Go net/http，已固定 go.mod/go.sum 并完成 Windows 构建；SQLite 依赖尚未引入。DNS 维护版/后继 0.x 取舍见 SOURCE_REVIEW.md。直接调用 typed HTTP API，不引入大型 SDK。

## 同步模式数据流

~~~mermaid
flowchart TD
  Q[可观察 DNS] --> R[标准化与硬规则/人工/可信规则/缓存]
  R -->|已知| D[正常回答]
  R -->|UNKNOWN_DNS| J[同域合并 + 有界 Jev 请求]
  J --> P[响应校验 + 证据门槛 + 版本核对]
  P -->|可采纳| C[单 writer 完整 snapshot]
  C --> A[provider PUT ACK + 元数据回读]
  A --> D
  P -->|超时/错误/UNCERTAIN| F[明确临时 fallback]
  F --> D
  D --> V[新连接 /connections 审计]
~~~

async 在首次回答后启动后台评估，不保持图中的同步顺序。原型日志记录 hash host_id、raw choice、accepted、放行来源；status 提供 mode/generation/聚合计数。connection identity 由人工 smoke 读取隔离 core 审计。

## 策略与缓存

Hard（LAN/private/local/必要 REJECT）> Manual > Explicit > Trusted > Learned > fallback。必要 REJECT 不强行映射三态。完整规范 hostname 是缓存和精确 DOMAIN 作用域，注册域仅作特征。特殊域/IP/非法 IDNA/私网回答先处理，模型不能覆盖。

UNKNOWN 只相对受支持、与 core 对齐的域名规则；原型只接收三种静态域名规则，其他类型拒绝配置。未来完整订阅中的进程/端口/IP/GeoIP/逻辑条件无法在 DNS 判定时应标 NOT_DECIDABLE_AT_DNS，不能忽略。输入仅含最少 hostname 和必要事实，不上传页面、URL path、历史、节点或 Controller secret。

无证据/冲突、非法概率、低分、timeout/429/5xx 均 UNCERTAIN。不用 1-P(PROXY) 当 DIRECT。样例 0.8 门槛未经校准，不是上线阈值。Economy fallback DIRECT 是明确策略，不是模型确认直连。

SQLite：hostname、decision、source、model_version、scores、created_at、expires_at、last_seen，另有 ruleset_version、policy_version、status、applied_generation。人工永久；learned 7 天只是初始上限；UNCERTAIN 短内存 TTL；fallback 不持久化成路由知识。

同 hostname 的 A/AAAA/HTTPS 只共用决定，各自保留 DNS 语义。缓存容量、API 并发/队列、每日请求/费用有限。热路径不自动重试；持续错误简单熔断、冷却后单请求恢复。

## 提交与期限

pending → 原子发布完整 snapshot → provider PUT → ACK/回读 → applied → 放行同步请求。不同 hostname 由单 writer 合并。启动/core 重启/profile 切换/TTL 清理均重建 provider，DB 有记录不等于已应用。当前两个 learned provider 没有统一事务；人工规则由 render 静态输出。

初始实验 preflight 预算 1.5 s，包含排队/API/提交；上游 DNS 可并行，整个 DNS query 另设绝对 deadline。生产 Go 用 context/HTTP client 绝对期限，不能用可反复刷新的 socket timeout 替代。Python benchmark 的 8 s socket timeout 不是产品实现。

TLS keepalive 降低建连成本，但休眠/断网会重新建连。原型 API 并发上限 2、候补 2，已做边界测试；此次 4 fresh requests 不等同产品排队配置。关闭/熔断/队列满即 fallback。Gate 自身死亡需外部 supervisor 和旁路，进程内 fallback 无效。

Go RSS 30–100 MiB 是产品预算，无模型显存；当前轻负载样本空闲 working set 11.70 MiB、采样峰值 16.79 MiB。模块保持小函数、显式状态/原因链，不建 Dashboard、ORM、事件总线或驱动。
