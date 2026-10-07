# 当前环境与兼容性

公开脱敏版本。硬件/安装信息来源于 2026-10-06；Controller 在 2026-10-07 只读刷新。未修改现用 FlClash、DNS、TUN、浏览器策略或订阅。原始快照仅本地保留。

| 项目 | 已观察状态 |
|---|---|
| 系统/硬件 | Windows 11 amd64；Ryzen 5800H，8C/16T，16 GB；RTX 3060 Laptop 6 GiB |
| FlClash | 0.8.99+2026100301（10-06 核实） |
| bundled core pin | chen08209/Clash.Meta@8597778c7df410ebe88c57e59eaab32d203156df |
| 活动 /version | meta=true, version=1.10.0；不可当作官方 release 身份 |
| 10-07 Controller | 回环 HTTP 可读，未做写入 |
| mode/TUN | rule；TUN enabled，Mixed，any:53 hijack |
| 10-07 活动规则 | **610 条**，含域名、IP、GeoIP、进程与 MATCH |
| 10-07 providers | 0；现用 bundled core provider 写入未验 |
| 隔离 core | 官方 Mihomo v1.19.32，与 bundled core 分开 |
| Jev 出站 | 显式进入现用 mixed-port；未证明物理外网链路无代理/TUN |

昨天活动规则只有一条 MATCH，今天已变为 610 条，不能继续把“一条规则”当作当前阻塞。仍需确定订阅更新/覆写/Agent matcher 的一致语义。DNS query 不能复刻进程/IP/GeoIP 条件。

10-06 显式 Mihomo/TUN 查询返回 Fake-IP，系统解析和其他 resolver 入口存在差异；还有虚拟网络及分流 DNS。今天未全量复测，不作为当前整机接管证明。浏览器有效 Secure DNS 状态仍未验收；持久偏好缺少显式值不等于关闭。

API 不需要模型常驻。Go Agent 30–100 MiB 是预算，实际 RSS/idle CPU/磁盘/冷启动待开发后测量，且不包含原有 FlClash/Mihomo。
