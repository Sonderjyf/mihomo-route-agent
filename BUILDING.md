# 构建与运行 Go 原型

当前版本 `0.1.0-dev`，用于独立 Mihomo 的机制验证。默认 `mode=off`。不要把样例的实验端口直接替换为日常 FlClash Controller；Agent 启动后会刷新其 Controller 上两个同名 provider。

## 构建

需要 Go 1.24 或更高。当前在 Windows amd64、Go 1.25.10 构建验证。主程序没有 Python、CUDA 或数据库运行依赖。

~~~powershell
go mod download
New-Item -ItemType Directory -Force dist | Out-Null
go build -trimpath -o dist/route-agent.exe ./cmd/route-agent
./dist/route-agent.exe version
./dist/route-agent.exe explain --config config.example.json direct.route-lab.test
./dist/route-agent.exe render --config config.example.json
~~~

`render` 输出 JSON 形式的 Mihomo 配置片段；只含 DNS、两个 provider 与受支持的规则序列。它不是完整配置，也不自动合并订阅、创建 PROXY 组、安装服务或改变系统 DNS。`explain` 只检查配置中的静态规则，不读取运行时学习缓存或调用 API。

## 配置与生命周期

复制 `config.example.json` 为忽略的 `config.local.json`。样例 `upstream=127.0.0.1:15356` 是实验 DNS 占位，须有独立 UDP/TCP DNS 服务；上游不能回到 Gate。`controller=127.0.0.1:19091` 是隔离 core；`api_proxy=127.0.0.1:7888` 是显式 API 出口。全部监听只接受回环地址。

| 参数 | 行为 |
|---|---|
| `mode` | `off` / `async` / `bounded-preflight`；默认关闭模型 |
| `fallback` | `DIRECT` 或 `PROXY`，由生成规则最后的 MATCH 执行 |
| `preflight_ms` / `dns_deadline_ms` | 决策整体预算 / 单 query 的整体预算；后者包含上游 DNS |
| `max_api_requests` | 本次进程最多 API 调用数；重启重置，不是每日费用上限 |
| `capacity` / `learned_ttl_seconds` | 有界内存学习表容量 / TTL；默认 1024 / 7 天 |
| `rules` | DOMAIN、DOMAIN-SUFFIX、DOMAIN-KEYWORD；manual > explicit > trusted，同层按输入顺序 |
| `judge` | `jev`；stub 仅允许显式 `--allow-lab-fixtures` |

关闭模型也会启动 DNS/provider 并尝试清空、同步同名 learned provider。学习表只在内存，重启清空；core/provider 磁盘缓存不能当作持久数据库。

~~~powershell
./dist/route-agent.exe serve --config config.local.json
# 在另一个终端查看
./dist/route-agent.exe status --config config.local.json
~~~

Jev 模式从 `OPENROUTER_API_KEY` 环境变量读取 key；Controller 从 `MIHOMO_SECRET` 读取 secret。不要把值写入仓库或命令参数。必须从外部向进程环境提供，主程序不读取 dotenv。空 `api_proxy` 忽略系统 HTTP 代理环境变量，但 TUN 是否接管仍取决于真实网络。API/节点解析的独立 bootstrap 需在接入前验证。

无实测可达性事实时，策略固定弃权为 UNCERTAIN；当前没有真实 TLS probe 接口。因此非实验模式不能产生新的 learned DIRECT/PROXY，即使 Jev 返回高分。它可验证静态规则、DNS 转发、限额和弃权流程；真实未知域名增益属于下一阶段。

HTTP `/status` 返回聚合计数和 Go heap（不是进程 RSS），`/rules/learned-direct.yaml`、`/rules/learned-proxy.yaml` 提供精确 DOMAIN YAML。日志使用 hostname 哈希标识；哈希不是匿名化保证，公开日志仍需审查。按 Ctrl+C 结束前台程序。没有外部 supervisor，Gate 退出后 DNS 自动旁路尚未实现。

## 独立 smoke

下载 [官方 Mihomo v1.19.32](https://github.com/MetaCubeX/mihomo/releases/tag/v1.19.32) 到自己的工具目录。脚本启动两个独立、无 TUN 的回环 core，以及实验 DNS/echo；仅终止自己创建的进程。需空闲端口 15354–15356、17891/17892、18081、18765、19091。

~~~powershell
python -m venv .venv
./.venv/Scripts/python.exe -m pip install -r requirements-smoke.txt
./.venv/Scripts/python.exe scripts/smoke_go.py --mihomo C:/tools/mihomo.exe --judge stub
./.venv/Scripts/python.exe scripts/smoke_go.py --mihomo C:/tools/mihomo.exe --judge jev --output local-evidence/go-smoke-jev.json
~~~

Jev smoke 使用环境中的 key，也可加 `--key-file C:/private/openrouter.env`（dotenv 的 `OPENROUTER_API_KEY`）；只发两个合成 .test API 请求，产生实际费用。`--proxy` 默认回环 7888，必须提供可用的显式出口。stub 完全离线。脚本开启的 `lab_fixtures` 被限定为 .test，并明确标注 synthetic；不读取浏览历史，不探测网站。

两种后端复用同一个 smoke：A/AAAA/HTTPS 合并判断 → 两个 provider PUT ACK 与数量回读 → DNS 放行 → SOCKS hostname 真实连接 RuleSet 命中；已知/私网不调用模型，私网连接 DIRECT；无证据不写入；50 ms 超时与迟到丢弃；Fake-IP 查询 Gate 命中为零。它不证明现用 FlClash/TUN 接入。

## 最小开发检查

~~~powershell
go test ./...
go vet ./...
go test -race ./...
~~~

Windows race 检查需要可用 C 编译器及 CGO；普通构建不需要。仅保留五组必要边界测试。真实 API、网站和付费实验不进入自动 CI。最终实测见 [PROTOTYPE_REPORT.md](PROTOTYPE_REPORT.md)。
