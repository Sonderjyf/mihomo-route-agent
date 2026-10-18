# 可行性与子系统判定

2026-10-07：轻量 API 候选值得继续验证；尚不能部署为整机默认 preflight。

| 子系统 | 判定 | 依据 / 下一道门槛 |
|---|---|---|
| Go 控制器、静态人工规则、内存缓存 | GO（隔离原型） | exe 10.41 MiB；空闲 11.70 MiB / 采样峰值 16.79 MiB；SQLite/服务恢复待开发 |
| Jev 官方 API 访问 | GO（当前路径） | 主实验 57/57 成功；不代表长期零失败 |
| 三态合成证据分类 | GO（契约样本） | 12/12；无证据/冲突弃权 |
| Jev 凭域名判断实时网络 | NO-GO，未验证 | 没有真实网络 ground truth，云端不是事实源 |
| Go API + Gate + provider | GO（隔离机制） | 真 API 一次调用，ACK < DNS 放行 < TCP 连接；RuleSet 命中，私网实际 DIRECT |
| 现用 FlClash/TUN 首次连接 | 待验收 | 尚未注入 Gate/provider；IP→hostname/DoH/代理入口须分开验 |
| 原 300 ms preflight | NO-GO | warm P95 539 ms；fresh burst 最大 1172 ms |
| 有期限 preflight | CONDITIONAL GO（实验） | 1.5 s 初始预算，含排队/API/提交；不是保证 |
| 异步 API、下次新连接生效 | GO（原型边界） | 独立有界后台任务已实现；生产路径待验，不保证首次连接 |
| API 独立 bootstrap | 待验收 | 沿用现有代理不证明上线后无递归 |
| Laya 默认常驻 | 不推荐 | RSS 1.76 GiB、CUDA allocator peak reserved 2.46 GiB |
| 普通 Fake-IP + 上游 Gate | NO-GO | 隔离查询 0 次进入 Gate |
| Hybrid Fake-IP / WFP / WinDivert | Deferred | 首版不扩大为全连接拦截 |

最终 Go 版本见 [PROTOTYPE_REPORT.md](PROTOTYPE_REPORT.md)：真实 Jev 首次 DNS 约 1.18 s，硬规则/私网、期限和真实 hostname 连接通过；仍是合成事实。

## 早期 Python 闭环范围

first.route-lab.test 三种 QTYPE 并发；Jev 输入标注 synthetic fixture，提供“重复直连失败、代理成功”的合成事实。最终复验 PROXY 概率 0.99，API 1045.5 ms，DNS 约 1.06 s；provider PUT 204，ruleCount=1，然后释放 DNS、再建立 hostname SOCKS5 连接。

实际连接经过第二个 core 的 SOCKS hop，/connections 为 RuleSet / learned-proxy / [lab-hop, PROXY]。[证据](evidence/gate-jev-2026-10-07.json)

此实验没有完整硬规则 matcher；回环目标仅用于控制面测试，不代表允许模型覆盖 LAN。它不是实时探测、TUN 验收或长期可靠性报告。

## 生产接入门槛

1. 自有规则栈与 matcher 对齐，不支持的条件规则不可当 UNKNOWN。
2. API/节点 DNS 独立 bootstrap；API 失败仍能 DNS/fallback。
3. 独立 FlClash profile 验证真实 TUN 新连接，再验订阅刷新和 core 重启。
4. 少量真实未知域名证明模型增益。现有合成标签直接可用代码判断，12/12 不证明需要 AI。
5. Gate 死亡有可测外部恢复/旁路，才允许日常 DNS 接入。
