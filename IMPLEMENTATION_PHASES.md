# 实施阶段与最小测试

核心流量闭环未验收前，不开发 Dashboard、训练或复杂探测。GitHub 先用于规划和实验管理，不发布可安装 alpha。

## 阶段 0：资源与 API 评估（本次完成）

既有离线 Laya benchmark，新增 Jev 40 热请求、4 fresh 并发、12 合成契约、真实 Jev 隔离 Gate/provider/SOCKS 闭环；3 个离线实验客户端边界测试。已发现原 300 ms 预算不成立。没有部署生产 Agent或修改现用网络。

验收范围：当前 API 可用和隔离控制机制；真实未知域名增益、现用 FlClash/TUN 整链仍未证明。

## 阶段 1：最小 Go 集成原型（隔离机制完成，FlClash 接入待验）

已实现 DNS UDP/TCP、固定规则清单、singleflight、HTTP provider、Controller、明确 fallback 与绝对期限；五组测试、vet/race、真实 Jev 隔离 smoke 通过。缓存暂存内存，不加数据库/服务安装器/UI；未知静态规则类型拒绝加载。详见 [原型记录](PROTOTYPE_REPORT.md)。

stub 和真实 Jev 共用同一 Go smoke，已验证独立官方 core 的 hostname 连接和私网保护。待验：独立 FlClash profile 正常预览/应用；分别验显式 hostname 与 TUN IP，新连接实际命中、API bootstrap 无递归、订阅刷新可重复。失败停在这里。

同步模式先 1.5 s 实验预算。async 已有独立有界后台任务并通过边界测试，是建议正式默认；尚未做生产路径验收，不可拿异步结果满足首次必判成功标准。

## 阶段 2：小样本增益与失效验证

最多 10–20 个经人工授权/选定的真实未知域名，记录时间、证据和路径，先 shadow。直连路径必须证明绕过业务代理/TUN，代理路径核对实际 chain；不要把通过代理的探测算成直连。只做必要 DNS/TCP/TLS，不 GET 页面。

比较规则+fallback 基线；若模型只根据 hostname 猜测或无增益，保持关闭/仅人工规则，不降低阈值制造闭环成功。synthetic 12/12 不能代替这个门槛。

验 API timeout/429/5xx、断网/节点失败、迟到结果、重启和 Gate 外部恢复；总 DNS deadline 必须覆盖排队与提交。

## 阶段 3：本机 CLI

加 SQLite、有界缓存、TTL/provider 清理、版本失效、人工 Force Direct/Proxy/Reset、health/status/explain、简易熔断和费用限额。不需要账号自动充值或 key 管理平台。

重新实测 Go RSS/CPU/磁盘/启动，分别报告模型关闭、缓存命中、少量 UNKNOWN 与受控 burst。预算 30–100 MiB 不是预先验收结果。重启后 applied 与 core 对齐，过期项真的从 provider 删除。

## 阶段 4：正式使用门槛

只在真实路径/恢复/订阅更新通过后发布小版本。默认模式与覆盖边界写清；CLI 明确异步只影响未来新连接。当前公开仓库已有 Go 源码、文档、实验和脱敏证据，没有公开 alpha 二进制。

## 最少测试预算

生产实现仅保留 5 组表驱动单元边界 + 1 人工真实 smoke + 1 人工 API benchmark：

| 组 | 必要内容 |
|---|---|
| Domain | 规范化、IDNA/PSL、IP/localhost/local/非法输入，子域不扩散 |
| Precedence | Hard > Manual > Trusted > Learned，unsupported 不伪造 UNKNOWN |
| Policy | 三态/无证据/低分/非法响应/timeout，fallback 不学习 |
| Singleflight/deadline | A/AAAA/HTTPS 共用决定，取消/排队/迟到与两种模式 |
| Compiler/commit | 排序/去重/DOMAIN、并发不丢更新、部分失败/TTL/core重启 |
| Integration | API 一次→provider ACK→DNS→真实连接；API死/Gate死必要分支 |
| Benchmark | 热 40次+4 fresh，短样本分类；手动、有预算、不进 CI |

现有 scripts/test_verify_jev.py 仅 3 个实验边界方法；Go 使用约定的五组必要测试，不堆叠重复套件。付费 API smoke 仍人工执行，不建立自动网站/付费实验流水线。

不追 coverage，不做 getter/CRUD 大量测试、GUI automation、真实网站自动 CI、大型模型准确率平台、全协议矩阵或压力平台。

## 项目管理与停止条件

少量 Issues 对应三个下一步门槛：FlClash/TUN 集成、真实小样本增益、Go 资源/恢复。只保留 integration/model/docs/bug 类别；不需要复杂 Projects 自动化。

最终规则无法归属、TUN hostname无法关联、模型无增益、deadline持续超标或Gate无可靠旁路时，停止扩大使用。公开内容白名单不含本机原始配置、浏览历史、节点、key、DB、权重/二进制；所有原始快照留忽略的 local-evidence/。
