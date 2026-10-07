# 公开项目源码与维护评估

2026-10-07 更新：下列是 10-06 固定源码调查，未声称今天重新审计全部上游。当前默认候选已改为 Jev HTTP 后端；Laya/local-jev 仅历史对照，维护判断不等于实际路由质量。新的 API 证据见 API_MODEL.md。

调查日期 2026-10-06。读取指定仓库源码、相关 LICENSE、GitHub API commit/release 状态。表中活跃仅表示可见维护证据，不是第三方可靠性背书；没有 fork 或运行这些参考产品。

## 选择矩阵

| 项目 | 已核对快照/维护证据 | 复用结论 |
|---|---|---|
| FlClash | v0.8.99；本机对应 68c71b8；10-03 release | 现有宿主；复用订阅/节点/TUN/生命周期，不 fork |
| Mihomo Go 内核 | Meta 88dcbf7；v1.19.32，09-30 release | 复用规则与 Controller，不嵌入整套 Go 内核到 Agent |
| FlClash bundled core | v0.8.99 pin 8597778 | API/配置兼容性必须单独验收，不能等同 upstream release |
| liuzq1103/FlClash-rules | 73d1520，09-04；08-21 创建；未见 release | 小型个人规则工程，借鉴订阅级幂等脚本，不作为成熟依赖 |
| charley008/mihomo-rule-inspector | 96f9820；v0.1.2，06-11；可见创建/更新集中一天 | 参考 connection 采样；不复用 UI/secret 行为，不宣称持续活跃 |
| Uddoo/mihomo-smart-selector | 3cd4991，09-29；v0.4.2；09-02 创建 | 参考 Controller client/SQLite/回读；活跃但年轻，非成熟网络基础设施 |
| IrineSistiana/mosdns | 9cfb7ce，02-27；v5.3.4，01-11 | 成熟 DNS pipeline 参考；更新较慢不等于无人维护；不引入整套插件框架 |
| LeonaDavinci/laya-system-one | d120d4b，09-23；fork，0.3.9 | 原 prompt 链接落后，runtime 改用实际上游 |
| NandhaKishorM/laya | a4a8921，10-05；v0.3.28 | 活跃，但项目年轻、变动快；固定版本，本机实测后使用 |
| amithgc/local-jev | 64a0b31，09-21；未见 release | 本地 typed API/模型卡参考；WSL2 路径/更多依赖不适合本机首版 |
| TypeSafeAI/typesafe-router | 4c6855c，09-26；未见 LICENSE | 非官方社区示例、private npm package；只借鉴职责边界，不复制代码 |
| Safing/portmaster | 882d900，10-06；v2.2.3，08-14 | 阅读 WFP/ALE 等待与重注入设计，首版不使用驱动 |
| basil00/WinDivert | 9710107，2022-04；v2.2.2，2022-09 | 成熟但可见发布较旧；未来评估驱动兼容性，不称为近期活跃 |

GitHub `pushed_at` 不等于默认分支最后一次代码提交，fork 的同步时间也不等于有效改进；上述判断没有混用这两个字段。

特别注意：此次 MetaCubeX/mihomo 的默认 `main` 分支实际是无关的 Python 数据客户端内容，API 的根 license 显示 MIT；**真正 Go 内核在 Meta 分支，LICENSE 是 GPLv3**。不能盲信默认分支、仓库根元数据或名字相同。FlClash bundled core 也必须读 `FlClash`/指定 commit，不能读它的默认 main。

## 已读源码与具体结论

### FlClash

读取 `lib/providers/actions/setup.dart`、`lib/common/task.dart`、`lib/common/path.dart`、`core/common.go`、`.gitmodules`。

确认 script 在应用补丁之前运行、provider 路径会重写、DNS override/appendSystemDns 会改变结果、文件路径归属与内核实际 apply 流程。需要从正常应用层配置 controller，不通过脚本假装完全拥有客户端字段。[固定版本源码](https://github.com/chen08209/FlClash/tree/v0.8.99)

### Mihomo

读取 `dns/middleware.go`、`dns/enhancer.go`、`tunnel/tunnel.go`、`hub/route/provider.go`、`rules/provider/provider.go`、`component/resource/fetcher.go`；并对照 FlClash pin。

确认 fake-ip 提前返回、IP→host 反查性质、PUT 更新等待 onUpdate、单 provider GET 不存在，以及 FlClash embed mode 限制整配置更新。隔离 smoke 对其中几个关键点提供了实测，不是只据 README 推断。[Meta 源码](https://github.com/MetaCubeX/mihomo/tree/88dcbf7f1614a67c3b36b848ee3592dfa92ada36)

### FlClash-rules

读取 `templates/override.js.tpl` 与 README 的客户端部署路径。复用“根据当前订阅节点动态生成组、给 profile 关联脚本、异常恢复”的做法。其现有设计仍保留订阅基础规则/MATCH，与我们完全受控规则栈的目标有差异，不能直接复制完成任务。[固定源码](https://github.com/liuzq1103/FlClash-rules/tree/73d152075c567f3d8b0c853d1bccc9d5910716dd)

### mihomo-rule-inspector

读取 `internal/probe/probe.go`、`internal/mihomo/client.go`、`internal/server/handlers.go`、`go.mod`。

可借鉴先打开连接流、维持请求、按 host 收集 rule/rulePayload/chains；生产关联应增加源端口/时间，避免同 hostname 多连接串样。其 config handler 返回 `secret`，我们不复制；它使用 Wails，亦不是值得引入的最小 CLI 依赖。[固定源码](https://github.com/charley008/mihomo-rule-inspector/tree/96f982089c1b8251f9eef962383ddb42e813e4c5)

### mihomo-smart-selector

读取 `internal/mihomo/client.go` 与相关安全/配置代码、`go.mod`。复用服务端持有密钥、固定/受限目标地址、禁止环境代理和重定向、请求 timeout、操作后回读。SQLite pure-Go 路径适合 Windows。UI、扫描、服务指标、自动节点切换不在本项目首版范围。[固定源码](https://github.com/Uddoo/mihomo-smart-selector/tree/3cd4991cb387a17ddd119fab44cf32315f94ef86)

### mosdns

读取 `plugin/executable/sequence/sequence.go`、`plugin/executable/forward/forward.go`、相关 server/cache 目录与 go.mod。借鉴 request context、有限超时和转发职责，不复制 Prometheus/插件注册/多协议全家桶。源码 GPLv3，不把它的实现直接复制进计划采用 MIT 的代码。[固定源码](https://github.com/IrineSistiana/mosdns/tree/9cfb7ce985599c087cb7ccfb1531d0c0f4021242)

### Laya

对比 fork 0.3.9 与上游 0.3.28 的 `pyproject.toml`、`laya/agent.py`、`router.py`、backends 与 LICENSE。核对 checkpoint safetensors、encoder config、temperature 配置与模型卡。

实际使用 eager backend、固定本地路径、单 checkpoint；记录 CUDA/CPU 实际 device，防止静默 CPU fallback 被报成 GPU 数字。温度警告、概率字段差异和域外适用性进入 policy 设计。[上游固定源码](https://github.com/NandhaKishorM/laya/tree/a4a8921afebfd852bba0000475cfb6ab737a124c)

### local-jev

读取 `src/local_jev/engine.py`、`backends/nli.py`、模型卡目录、pyproject 与 LICENSE。借鉴模型 registry、设备锁、明确精度/内存边界；不复用其多模型加载管理或完整 web server。公开延迟来自其他硬件，不用于本机结论。[固定源码](https://github.com/amithgc/local-jev/tree/64a0b31ff343dca32142496cf9edca5a0174a18d)

### TypeSafe / Portmaster / WinDivert

读取 TypeSafe `lib/jevRouter.ts`、`guards.ts`；只借鉴“候选选择→校验→fallback→宿主执行”，不把社区项目误称官方 SDK。[源码](https://github.com/TypeSafeAI/typesafe-router/tree/4c6855ccfc92ff0e40a71661685c9ead327a3715)

读取 Portmaster `windows_kext/README.md`、`PacketFlow.md`，了解 WFP/ALE pending 与 verdict；WinDivert 官方文档和 LICENSE 确认用户态截包/重注入与驱动依赖。WinDivert 是 LGPLv3/GPLv2 双许可选择，不是 GitHub NOASSERTION 就“没有许可”。没有驱动实现/安装。

## 基础库的维护取舍

- `github.com/miekg/dns`：09-01 仍有提交，但 README 已声明只接受特定修复、未来归档；首轮固定维护版，不能宣传成积极功能开发。
- 后继 `codeberg.org/miekg/dns@93e4bc7f...`：10-04 更新、BSD-3-Clause，已读 README/go.mod/client.go/server.go；Go 1.27、0.x API 与更多模块依赖。保留迁移路径，不直接相信“2 倍速度”适用于 Windows。
- `go.yaml.in/yaml/v3` 对应 go-yaml 项目 09-30 有更新；不使用已归档的旧仓库作为活跃维护证据。
- singleflight、IDNA/PSL 用 Go x/*；SQLite 用 modernc。按首版最低工具链与已验证版本 pin，不自动追 main。

## 复用与许可边界

FlClash/Mihomo/mosdns/Portmaster GPL；Laya Apache；FlClash-rules/inspector/selector/local-jev MIT；TypeSafe 未发现明确授权文本。运行独立程序、调用已公开 API、阅读设计、复制/链接源码与重新分发二进制是不同使用方式。首版只将必需库作为依赖，不复制整套 GPL 项目、不打包别人的驱动；以后如改变分发方式，再审查具体义务。
