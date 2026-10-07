# 离线接入预览：当前支持范围

`preview` 是里程碑 1 的首个离线增量，**不是完整 FlClash 集成**。它不查找用户目录，不读取活跃订阅，不启动 core，不连接 API，不应用配置。只读取显式传入的 Agent 配置和 JSON profile 副本，输出可检查的计划文件。

当前支持一个刻意收窄的实验 profile：静态 IP 的 SOCKS5 节点、直接引用节点或 DIRECT 的 select 组、DOMAIN/DOMAIN-SUFFIX/DOMAIN-KEYWORD 与末尾 MATCH。只允许回环监听、rule 模式、redir-host、关闭 TUN。GeoIP/进程/端口/逻辑规则、第三方 rule-provider、动态 proxy-provider、嵌套组、Fake-IP、其他 DNS 策略或未知字段会明确报错，不静默忽略。

不要为了让预览通过而删改日常配置。先使用仓库内的无凭证 fixture；日常 FlClash 导出的配置通常会超出此子集，需要后续的规则语义适配与 YAML/overwrite 支持。当前 CLI 只接受 UTF-8 JSON，不接收 YAML。

## 生成并查看计划

在仓库根目录执行；这些步骤只写入新的本地文件：

~~~powershell
go build -trimpath -o dist/route-agent.exe ./cmd/route-agent
New-Item -ItemType Directory -Force local-evidence | Out-Null
./dist/route-agent.exe preview --config config.example.json --profile examples/isolated-profile.json --output local-evidence/preview.json
$plan = Get-Content -Raw -Encoding UTF8 local-evidence/preview.json | ConvertFrom-Json
$plan.changes
$plan.limitations
~~~

`--output` 必须是不存在的新文件；重复运行或把它指定为源 profile 都会拒绝覆盖。不指定该参数时输出到 stdout。计划含源文件 SHA-256、`candidate`、`agent_config`、逐字段 before/after 差异、管理字段说明和回退步骤。输入若有节点凭证，候选也会保留它们；只在本地查看，不上传整份计划。

候选仅改写 `dns.enable/enhanced-mode/nameserver/use-hosts/use-system-hosts`、`rule-providers` 和 `rules`。原始文件、节点、组、端口与其他已支持字段保持不变。原 profile 的域名规则按原有顺序成为配套 Agent 的 explicit 规则，配置中的 manual 仍优先；随后由同一编译器生成 core 规则，避免 Gate 与 core 对已知域名产生不同理解。源 MATCH 必须与 Agent fallback 一致。

必须将 `candidate` 与计划中的 `agent_config` 配套使用，不能只拿候选配置搭配旧 Agent 配置。以下命令只是提取审查文件，不会运行服务；请选择未使用的文件名：

~~~powershell
$utf8 = [System.Text.UTF8Encoding]::new($false)
[System.IO.File]::WriteAllText("$PWD/local-evidence/candidate.json", ($plan.candidate | ConvertTo-Json -Depth 100), $utf8)
[System.IO.File]::WriteAllText("$PWD/local-evidence/agent-preview.json", ($plan.agent_config | ConvertTo-Json -Depth 100), $utf8)
./dist/route-agent.exe check --config local-evidence/agent-preview.json
./dist/route-agent.exe preview --config local-evidence/agent-preview.json --profile local-evidence/candidate.json --output local-evidence/preview-again.json
~~~

再次预览的候选与配套 Agent 配置保持一致；源 SHA 随输入文件变化。已存在的 managed provider 必须与配套 Agent 的生成内容完全匹配，否则拒绝接管。

## 拓扑检查与剩余验收

检查 Agent DNS/HTTP、core DNS/mixed/controller、upstream 的显式地址，拒绝端口冲突、IPv4-mapped 地址绕过以及 upstream 指回实验 core DNS。API HTTP 代理需独立于这些端点，启用模型时必须显式提供；节点必须是 IP，且不能回指这些端点。此检查不查询 DNS、不占用端口，也不能证明外部 resolver 或代理内部没有间接回环。

fixture 使用 `15354/15355/15356`、`17891/17892`、`18765/19091`，只是拓扑示例，不代表这些服务存在或端口空闲。当前没有自动应用/卸载工具；取消预览只需丢弃新产物，原 profile 无需恢复。

里程碑 1 尚欠：固定官方 core 版本的独立语法验收、真实 FlClash 导出/YAML 与 overwrite 适配、订阅刷新幂等和恢复验证。实际 TUN、真实 API bootstrap、shadow probe、生产 DNS 恢复均未因这个预览而完成。后续验收顺序见 [实施计划](IMPLEMENTATION_PHASES.md)。
