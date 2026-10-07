# 实施计划与验收门槛

目标是 Windows 上可运行、可解释、可撤回的 FlClash/Mihomo 路由助手。运行时采用 Go 单程序和可关闭的 Jev API，不引入常驻 Python、本地模型、CUDA 或训练流水线。正式启用模型后的默认选项是 **async**，bounded-preflight 仅供显式选择，off 始终保留。async 只改变未来连接，不承诺第一次访问就使用新规则。

当前仍是 0.1.0-dev 原型：示例和未指定 mode 的配置保持 off，避免尚未完成接入与恢复验收时自动发起收费请求。真实 TLS 证据采集尚未实现，正常模式的未知域名最终只能 UNCERTAIN。历史 synthetic smoke 只证明机制，不能替代真实网络收益。

## 当前增量：修复故障，补齐离线入口

| 交付 | 验收方法 | 边界 |
|---|---|---|
| 同域名请求在 provider 提交中继续共享等待 | 暂停首次 PUT，在 dirty 窗口发起 AAAA；ACK 前不得释放，ACK 后仅一次 judgment/commit | 覆盖 bounded-preflight，不改变 async 的立即 fallback |
| UDP 响应适配客户端大小 | 上游 UDP 返回 TC，TCP 返回大 TXT；检查无 EDNS、低于 512、1232、4096 与下游 TCP | 超限 UDP 带 TC，TCP 保留全部记录 |
| check --config 离线配置检查 | 端口、监听冲突、上游直接回环、Controller/API 代理 URL 的拒绝用例 | 不证明端口空闲、无间接 DNS 环路、凭证或真实链路可用 |
| 可复现构建 | go test -count=1 ./...、go vet ./...、go test -race -count=1 ./...、Windows build | 不调用真实模型，race 需要 CGO/C 编译器 |

这一增量不安装服务、不修改 FlClash、不发布二进制，也不宣称解决 provider 双文件事务或 TUN 身份问题。

## 里程碑 1：可审查的 FlClash 接入预览

首个离线子集现已实现：`preview --profile` 生成实验 JSON 候选和同步的 Agent 配置，拒绝不支持的规则/拓扑，保护原文件并支持重复预览。见 [使用范围与命令](PREVIEW.md)。这只完成子集的离线工具与验证，不代表下面的官方 core、真实 FlClash 或完整配置适配验收已经通过。

交付一个只处理显式输入副本的 profile 规划工具：读取原配置与 Agent 配置，生成完整候选配置、差异、Agent 管理的字段清单与回退说明。保留节点、组和原始规则；遇到进程、端口、GeoIP、逻辑规则等 DNS matcher 不理解的语义时停止并解释，不把它们当 UNKNOWN。API、节点 bootstrap 和 Gate upstream 必须有独立解析路径。

验收：固定 profile fixture 上重复应用两次结果相同；撤销候选变更可恢复原配置；非法配置和不支持语义不生成可应用结果；使用明确版本的官方隔离 Mihomo 验证语法。只处理本地副本，不自动打开系统代理/TUN 或修改活跃订阅。进入实际 FlClash 前，人工核对该版本的预览配置与活动 core API 是否一致。

## 里程碑 2：真实证据采集与 shadow 决策

先定义小型、可取消的 DNS/TCP/TLS probe 接口。直接路径必须证明绕过业务代理/TUN，代理路径必须验证实际出口与证书；只对用户授权目标检查连接和 TLS，不下载页面、不扫描网段。记录证据时间、有效期、失败分类和冲突；模型只能对这些证据提建议。

交付 shadow 模式：规则与明确 fallback 继续负责流量，Jev 建议只记录，不发布 learned 规则。限制目标数量、并发、超时和 API 总量；真实 API 测试必须显式开启并说明费用。预算耗尽和 API 异常不自动重试成风暴。

验收：离线假 probe 覆盖直连成功、代理成功、证据冲突、取消与过期；再对 10–20 个明确授权的真实未知域名建立带时间的人工基准，与规则+fallback 比较。报告正确/错误/UNCERTAIN 和实际连接成功率，不能用 hostname 高分或 synthetic 12/12 替代收益。未经独立路径验证，不进入自动学习。

## 里程碑 3：持久化、手动控制与失效恢复

证据和接入语义稳定后再引入 SQLite。记录精确 hostname、来源、证据、模型/策略/规则版本、TTL、期望状态和实际 applied generation；启动时先同步 core，数据库有记录不等于规则已生效。提供 Force Direct/Proxy/Reset 与明确 explain，人工选择优先于模型，fallback 不保存为知识。

验收：进程/core 重启、profile 切换、版本变化、TTL 过期、部分 PUT 失败、回滚失败、同数量但不同内容的 provider 均能被识别并恢复。报告恢复窗口；内容身份验证未补齐之前，不能把仅 ruleCount 相等写成“已确认一致”。使用独立 core 或假 Controller，不指向日常 FlClash。

## 里程碑 4：外部恢复与受控真实接入

先实现独立于 Gate 进程的健康检查和恢复方案：Gate 被终止后，DNS 仍有明确可执行的恢复路径。恢复操作依据接入前保存的状态，限制在 Agent 管理的字段并可撤销。通过隔离测试后，才在用户选择的实验 FlClash profile 执行，并准备可操作的回退步骤。

验收矩阵分别记录：显式 SOCKS hostname、redir-host 普通 DNS、TUN IP、浏览器 DoH/Fake-IP 未覆盖路径。核对真实 /connections 的 rule、rulePayload、chains，不只看模型日志。验证 API/节点 bootstrap 无递归、订阅刷新幂等、core 重启恢复、Gate 强制退出后的 DNS 恢复，以及长连接不会被误称为已迁移。**TUN 未实测就标未验收**，不能用 SOCKS 结果代替。

## 里程碑 5：小范围可用 CLI 发布

只有真实收益、接入、恢复三项均通过后才制作 Windows 发布包。启用流程明确选择 fallback 并配置 Jev 凭证；启用后的默认选项为 async，bounded-preflight 需理解等待预算后主动选择。凭证采用平台存储，状态/日志/导出不包含 key、订阅或浏览历史。

验收：干净 Windows 环境的安装、升级、卸载和回退；模型关闭、常态使用、短时间 UNKNOWN burst 下分别测 Agent RSS/CPU/队列/丢弃率和 DNS 延迟，按与基线比较后确定的门槛验收。当前 30–100 MiB 是预算，不是已有长期结果。输出已知限制与证据索引，先提供 CLI，不为演示提前做 Dashboard。

## 执行顺序与停线条件

每次交付推进一个可验证门槛：变更说明、最小回归、隔离验证记录、剩余风险。离线 Go 检查是每次代码变更的门槛；Python/Jev、官方 core smoke、真实 FlClash/TUN 分开执行，不能因为第一项通过就标记后三项通过。付费 API 和真实站点不进入默认 CI。

出现 DNS 回环、ACK 前错误放行、无法恢复网络、TUN hostname 无法确认、模型无可测收益或资源持续越界时，停止扩大使用范围并保留 off。下一次优先交付里程碑 1 的离线接入预览；probe 设计可以提前讨论，但生产接入不能绕过持久化、失效与外部恢复验收。
