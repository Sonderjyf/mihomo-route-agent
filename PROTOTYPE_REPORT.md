# Go 原型验收记录（2026-10-07）

阶段 1 的独立 Go/Mihomo 机制原型已完成，现用 FlClash/TUN 的整机接入仍未验收。主程序版本 `0.1.0-dev`，Windows amd64，Go 1.25.10；模块最低 Go 1.24。官方隔离 core v1.19.32，未使用现用 bundled core。

## 已实现

- UDP/TCP DNS Gate，截断 UDP 的 TCP 重试；普通 A/AAAA/HTTPS 共用同域 singleflight。
- IDNA/PSL、精确 hostname 缓存、硬规则和私网回答保护；manual > explicit > trusted > learned > fallback。静态 matcher 与 render 共用规则清单，未知规则类型拒绝加载。
- off/async/bounded-preflight；独立后台期限、2 workers + 2 候补、进程请求预算、30 秒弃权内存缓存。最后一个同步等待者取消后丢弃该任务；一名等待者取消不影响其他等待者。
- 两个精确 DOMAIN provider、单 writer 完整 snapshot、PUT ACK/数量回读、失败回滚、TTL 删除；每 5 秒检查 core 元数据，缺失/数量不符/失败则失效并尝试重同步。
- Jev typed HTTP client、三态证据门槛、hash 日志和 CLI serve/render/status/explain/version。

没有 SQLite、真实 TLS probe、服务安装/外部恢复、人工规则 CRUD、生产 profile 注入或 Dashboard。真实可达性事实尚未接入，所以普通模式无证据时始终弃权，不能按 hostname 高分自动学习。

## 最终构建的最小验证

`go test ./...`、`go vet ./...`、`go test -race ./...`、Windows 构建全部成功。五组测试覆盖 Domain、Precedence、Policy、Singleflight/deadline、Compiler/commit；包含并发更新、部分失败回滚、TTL/core 恢复及 async，不以覆盖率作为验收。

最终构建真实 Jev smoke 通过。A/AAAA/HTTPS 三条首次 DNS 共用一次 Jev，耗时 1175.7–1176.9 ms；provider applied 事件早于全部 DNS 放行，之后建立的 SOCKS hostname 连接命中 `RuleSet / learned-proxy / [lab-hop, PROXY]`。私网 hostname 的实际连接命中 `IPCIDR / 127.0.0.0/8 / DIRECT`。

TCP 缓存 DNS 约 1.0 ms；50 ms stub 超时分支约 54.2 ms 放行，后续没有 learned 项或提交；Fake-IP 模式 Gate 命中 0。无证据请求虽调用了 Jev，但没有新增学习规则。详情见 [合成证据 JSON](evidence/go-smoke-jev-2026-10-07.json)，包含 executable SHA-256，可和本地构建文件比对。

## 资源

| 指标 | 最终构建短时样本 |
|---|---|
| exe | 10,916,352 bytes，约 10.41 MiB |
| 空闲 Windows working set | 12,263,424 bytes，约 11.70 MiB |
| 20 ms 周期采样峰值 working set | 17,608,704 bytes，约 16.79 MiB |
| 约 1 秒空闲 CPU 增量 | 0 秒；该短样本不能推出长期零 CPU |

资源只包含 Agent，不含原有 FlClash、两个实验 core、Python 或其他系统进程；采样峰值可能遗漏瞬时峰值。此前 30–100 MiB 是产品预算，本次轻负载结果不是生产长期容量保证。

## 接入前仍需证明

现用 bundled core 写入、独立 FlClash profile 正常应用/订阅刷新、TUN IP→hostname、有效 DoH、API bootstrap、真实小样本增益和 Gate 死亡恢复都未通过。provider 数量回读也不证明同数量内容或 profile 身份；两个 provider 没有统一事务，core 变化检测有最多约 5 秒加请求超时的窗口，TTL 清理同样有周期延迟。

接入前须核对规则语义，特别是进程/端口/IP/GeoIP/逻辑条件；当前不支持导入完整生产规则。动态规则不会迁移已建立会话；async 只影响以后新连接。上述缺口保留在 Issues，不发布日常网络使用的 alpha。
