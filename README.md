# Mihomo Route Agent

现有 [离线接入预览](PREVIEW.md) 可从明确指定的实验 JSON profile 生成候选与配套 Agent 配置；当前仅支持静态 SOCKS5/域名规则子集，不是完整 FlClash 集成，也不应用任何设置。

本轮增量修复了 provider 提交窗口中的同域名提前放行，以及 UDP 大响应缺少截断的问题，并增加 `route-agent check --config ...` 离线检查。后续按 [实施计划与验收门槛](IMPLEMENTATION_PHASES.md) 推进：接入预览 → 真实证据 shadow → 持久化与恢复 → 实验 FlClash/TUN → 小范围 CLI 发布。目标采用 Go + Jev，启用模型后的默认选项为 async；当前原型示例继续 off。

面向 Windows + FlClash/Mihomo 的轻量自适应路由控制器。静态规则和人工设置优先，仅对未知域名评估 DIRECT / PROXY / UNCERTAIN，并通过动态 Rule Provider 保存可采纳结果。

**当前已实现 `0.1.0-dev` Go 原型，尚未接入日常 FlClash/TUN。** 默认模型关闭，启动见 [BUILDING.md](BUILDING.md)。使用 Go Agent + 可选 Jev API；Laya 不再作为默认常驻组件。控制器本地运行，推理需要联网。不 fork FlClash/Mihomo。

## 本机验证

| 验证项 | 结果 | 能说明什么 |
|---|---|---|
| Jev 40 次热请求 | 40/40 成功；P50 476 ms，P95 539 ms | 当前路径的短输入性能，不是 SLA |
| 新 HTTPS 连接 | 首请求 1.12 s；4 请求并发最慢 1.17 s | 建连开销必须计入首次等待 |
| 合成证据分类 | 12/12 符合预期 | 这组输入契约成立，不是实时路由准确率 |
| 最终 Go + 真实 Jev 隔离闭环 | 三种 QTYPE 共用一次调用；首次约 1.18 s；ACK 后 DNS 放行；连接命中 learned-proxy | 独立 core 的机制成立，合成证据 |
| 私网 hostname 连接 | 实际命中 DIRECT，未调用 Jev | 硬规则优先于学习规则 |
| Go 最小检查 | 五组边界、vet、race、Windows 构建通过 | 不代替生产网络验收 |
| 现用 FlClash/TUN 整链 | 未验收 | 不能宣称整机首次连接已覆盖 |
| 原 300 ms preflight | NO-GO | API 热 P95 已超过目标 |

主 API 实验 57 次调用，响应上报费用合计 $0.00113379。没有上传用户浏览历史，没有修改现用 DNS/TUN/订阅。证据使用合成 .test 域名和合成网络事实。

## 资源与运行方式

Go 单 exe + CLI 已实现，有界缓存暂存内存；SQLite 和服务恢复尚未开发。最终短时真实 Jev smoke：空闲 working set **11.70 MiB**，20 ms 周期采样峰值 **16.79 MiB**，exe **10.41 MiB**。这些是轻负载样本，不是长期容量保证；30–100 MiB 仍是产品预算。无模型显存，不需要 Python/CUDA/WSL2/Docker 常驻；Python 仅用于人工实验。

当前无真实 TLS probe，普通模式无证据时固定 UNCERTAIN，不会按 hostname 高分新增路由。实验采纳仅使用显式开启的合成 .test 事实。

建议正式版默认规则/缓存 + 明确 fallback，API 在后台评估，影响未来新连接。该模式不保证未知域名第一次连接使用新决定。可选 bounded-preflight 实验模式在期限内完成判断和规则提交才覆盖第一次连接，否则 fallback。两种模式不能混称。

## 文档

| 文件 | 内容 |
|---|---|
| [BUILDING.md](BUILDING.md) | 构建、配置、CLI、独立 Go smoke |
| [PROTOTYPE_REPORT.md](PROTOTYPE_REPORT.md) | 最终原型验证、资源与缺口 |
| [CURRENT_ENVIRONMENT.md](CURRENT_ENVIRONMENT.md) | 脱敏兼容性快照和当前规则状态 |
| [FEASIBILITY.md](FEASIBILITY.md) | 按子系统 GO / NO-GO |
| [ARCHITECTURE.md](ARCHITECTURE.md) | 模块、两种模式、缓存与策略 |
| [INTEGRATION.md](INTEGRATION.md) | 注入、API bootstrap、provider 提交与回滚 |
| [API_MODEL.md](API_MODEL.md) | Jev 测量、费用和有效性边界 |
| [LOCAL_MODEL.md](LOCAL_MODEL.md) | 本地模型历史评估，默认不启用 |
| [LIMITATIONS.md](LIMITATIONS.md) | 覆盖边界和未完成验证 |
| [IMPLEMENTATION_PHASES.md](IMPLEMENTATION_PHASES.md) | 实施门槛与最小测试 |
| [SOURCE_REVIEW.md](SOURCE_REVIEW.md) | 2026-10-06 固定源码参考 |

## 人工复现

API benchmark 只使用 Python 标准库。将自己的 key 放在 OPENROUTER_API_KEY 环境变量，或用 --key-file 指向本地 dotenv。不要在命令行传 key 本身。脚本只访问官方 OpenRouter，忽略环境代理；--proxy 指定明确回环 HTTP CONNECT 入口。启用 TUN 时，不指定 HTTP 代理也不能证明物理直连。

~~~powershell
python scripts/verify_jev.py --proxy http://127.0.0.1:7888 --count 40
python -m unittest discover -s scripts -p test_verify_jev.py -v
~~~

第一条是付费联网实验，共 57 次请求；只检查接口可加 --probe-only。每次 socket 超时 8 s，不是产品绝对 DNS deadline。结果写入忽略的 local-evidence/，不打印 key 或 API 错误正文。

Go smoke 复现见 [BUILDING.md](BUILDING.md)。以下保留早期 Python Gate 实验，使用官方 Mihomo v1.19.32、两个无 TUN 的回环内核、DNS/provider/echo 服务：

~~~powershell
python -m venv .venv
.venv/Scripts/python.exe -m pip install -r requirements-smoke.txt
.venv/Scripts/python.exe experiment_dns_gate.py --mihomo C:/tools/mihomo.exe --workdir lab-work --decision-backend jev --proxy http://127.0.0.1:7888 --output local-evidence/gate-jev.json
~~~

自行从 [Mihomo 官方发布](https://github.com/MetaCubeX/mihomo/releases/tag/v1.19.32)获取内核，不随仓库分发二进制。回环端口 15354/15355、17891/17892、18081、18765、19091 必须未占用；结束清理自有进程。stub 为默认实验后端，不能算作 Jev 验收。[evidence/](evidence/) 仅保存经检查的合成结果。

原创文件 MIT；参考依赖见 [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md)。不含订阅、节点、密钥、设备原始快照、数据库或模型权重。
