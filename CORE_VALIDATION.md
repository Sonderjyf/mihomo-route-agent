# 官方 core 隔离验证记录

日期：2026-10-07。复用此前实验下载的官方 Mihomo，不安装新 core、不调用日常 FlClash Controller、不改变 DNS/代理/TUN。所有运行目录都是新建实验目录，模型使用 stub，凭证环境变量从子进程环境移除。

## 版本与复现入口

- core 自报：Mihomo Meta v1.19.32，Windows amd64，Go 1.26.8，with_gvisor。
- core SHA-256：`04f8d7fc2b314771e1ecbf15951d59f0e1cb7914503c88cfb6972ff6a824e314`。
- Agent 构建：Go 1.25.10 Windows amd64，`go build -trimpath`；精确 executable SHA 在 [合成证据](evidence/core-offline-2026-10-07.json)。构建时源码处于本轮待提交状态，该哈希不承诺等于提交后重新构建的产物。
- Python smoke 复用已有 dnspython 2.8.0/PyYAML 6.0.3，不安装运行时依赖。正式 Agent 不依赖 Python；YAML 输入由固定的 Go YAML 库处理。

将 `preview` 产物中的 `candidate` 提取到新的实验目录，在 core home 下为两个 provider 放置空 `payload: []` 缓存文件，然后执行：

~~~powershell
# 两个参数都必须指向独立实验目录，不使用 FlClash 的活动配置目录。
C:/path/to/existing/mihomo.exe -t -d C:/lab/core-home -f C:/lab/candidate.json
python -B scripts/smoke_go.py --agent dist/route-agent.exe --mihomo C:/path/to/existing/mihomo.exe --judge stub --workdir local-evidence/core-smoke --output local-evidence/core-smoke.json
~~~

JSON fixture 与 YAML fixture 生成的候选分别通过 `-t`，退出码为 0。该命令只验证独立候选语法，不证明 FlClash 的应用后配置相同。普通 `preview` 不会自动执行 core 校验。

## 本轮结果

| 项目 | 结果 |
|---|---|
| 同域 A/AAAA/HTTPS | 三个响应均成功，合并一次 stub 判断和提交，约 153 ms |
| ACK 与 DNS 顺序 | provider applied 先于全部首次 DNS 放行 |
| 实际 SOCKS hostname 连接 | 命中 RuleSet / learned-proxy，chain 含 lab-hop / PROXY |
| 已知域名和私网 | 不追加判断，私网实际连接走 DIRECT |
| 无证据输入 | 不增加 learned 条目 |
| TCP DNS | 成功 |
| 50 ms 超时预算 | 约 52.7 ms 释放；无迟到提交 |
| Fake-IP | Gate 命中 0，记录为未覆盖路径 |
| 测试资源退出 | harness 只终止自己创建的子进程；监听服务正常关闭 |

另通过 `go test -count=1 ./...`、`go vet ./...`、`go test -race -count=1 ./...`，以及 smoke harness 的占用端口拒绝/子进程退出检查。端口预检在写配置和启动进程前运行，避免把已有服务误当成实验 core；这不代表长期占用锁或生产 supervisor。

## FlClash 适配判断与下一份输入

核对 [FlClash v0.8.99 setup.dart](https://github.com/chen08209/FlClash/blob/v0.8.99/lib/providers/actions/setup.dart) 与 [task.dart](https://github.com/chen08209/FlClash/blob/v0.8.99/lib/common/task.dart)：脚本执行后，客户端仍会应用自己的端口、TUN、Controller 设置，改写 provider 路径，并可能覆盖 DNS 或追加 system resolver。因此不能仅生成 `main(config)` 就宣称接入成立。

下一步需要用户明确提供一份脱敏的实际导出 YAML，保留规则类型、顺序、组引用和节点协议类型；替换节点地址、密码、token、订阅 URL 等私密值。同时提供 DNS override、appendSystemDns、当前模式/TUN、预期 fallback 和 Agent 应使用的组名。只基于副本做适配，不直接读取活动配置，也不要求用户先改设置。

当前静态 SOCKS5/域名规则限制是原型验证边界，不是永久产品约束。先根据真实结构确定需要保留的规则语义与出口依赖，再扩展 adapter；不能删除用户已有 GeoIP/进程规则来让预览通过。

尚未验收：FlClash bundled core（版本不能与官方 core 等同）、实际 TUN、订阅刷新、真实 API bootstrap、真实 TLS evidence、持久化与 Gate 外部恢复。这些结果不能作为日常启用许可。
