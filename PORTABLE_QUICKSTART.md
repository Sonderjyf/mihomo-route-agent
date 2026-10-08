# Windows 便携包：隔离验收通过，真实出口与模型待验证

适用于 Windows amd64。无需安装服务、系统任务或 Python。包内只有 Agent、公开示例、文档和合成验收证据，不含 Mihomo、FlClash、订阅、密钥或用户配置。当前产品版本为 `0.1.0-dev`；确切源码 SHA、构建环境见 `manifest.json`，每个文件的 SHA256 见 `SHA256SUMS`，ZIP 校验值另附。

2026-10-08 的真实临时 Windows 合成生命周期验收在 `39bea08f169d4098fd8884a097b628b03f35f9aa` 通过，耗时 289 秒：首次回落、后续学习、规则范围、维护清空/恢复、父子故障与独立空规则恢复均通过，进程/端点清理及网络基线检查通过。证据为 `evidence/windows-lifecycle-success-2026-10-08.json`。它使用独立测试二进制注入合成证据和路径，不等于真实出口或模型通过。后续文档/打包提交的 SHA 与实际验收 SHA 分别记录。

## 解压后先做离线检查

默认 `config.example.json` 为 `mode=off`，控制器和模型代理为空。没有自动启动脚本；双击程序只显示用法并退出。以下命令不联网、不读密钥、不启动核心、不修改网络：

```powershell
./route-agent.exe version
./route-agent.exe check --config ./config.example.json
./route-agent.exe explain --config ./config.example.json example.com
```

本包不自动生成或应用 Mihomo profile。不要将示例端口直接视为你的控制器或代理。只有在另行批准的隔离环境中，才填写已核实的地址、有效核心配置路径及物理接口编号。

## 运行前选择维护时段

复制 `examples/maintenance.template.json` 到私有目录，例如 `private/agent.json`。模板故意不能直接通过检查：先将四项占位值改为你选定的 `HH:MM`、`UTC` 或 `Asia/Shanghai`、1..86399 秒、`manual` 或 `unchanged`。例如 `start=03:20, timezone=Asia/Shanghai, duration_seconds=300, resume=manual` 仅说明格式，不是默认或建议时间。

- `manual`：窗口结束继续暂停，需要停止后明确建立新的会话。
- `unchanged`：只在配置、核心和所有权仍相同且再次核验后恢复；核心重启或真实配置变更需要新所有权。
- 窗口内看到状态 `paused_empty` 才说明已完成那次清空核验；仅到达预定时间不能作为刷新许可。

```powershell
# 完成填写后仍只是离线校验；stub 必须显式允许。
./route-agent.exe check --config ./private/agent.json --allow-lab-fixtures
```

模板的目标仅有 `example.com` 与 `www.cloudflare.com`。`stub` 始终保守返回 UNCERTAIN，不能用它证明真实非空学习。真实模型需要另行批准、将 judge 改为 `jev`，并使用既有授权凭据；不在文件中填写密钥。

## 已获另行授权的环境：最小启动、停止和恢复

先确认独占且认证的隔离核心、两个受控 HTTP tail provider、固定有效配置文件、独立直连/代理路径、私有会话目录及维护时段。`MIHOMO_SECRET` 仅由该环境既有安全机制提供到进程；Jev 还需既有 `OPENROUTER_API_KEY`。本包不读取或复制其他设备凭据、不设置长期环境变量。以下命令是操作说明，当前没有授权执行。

```powershell
# $EffectiveConfigPath 和 $DirectInterfaceIndex 必须由该隔离环境核实。
# 以下以仍不产生真实非空学习的 stub 配置为例。
./route-agent.exe prepare-apply --config ./private/agent.json --allow-lab-fixtures --exclusive-controller --profile $EffectiveConfigPath --output ./private/session-ownership.json
./route-agent.exe run-controlled --config ./private/agent.json --allow-lab-fixtures --allow-continuous --allow-owned-recovery --allow-controlled-apply --exclusive-controller --ownership ./private/session-ownership.json --state-file ./private/session-state.json --direct-interface-index $DirectInterfaceIndex --allow-external-probes
```

这是前台命令。正常停止按 **Ctrl+C**，等待父子进程退出及两个受控 provider 清空核验；不要直接关闭终端。状态可在另一个终端用 `route-agent.exe status --config ./private/agent.json --allow-lab-fixtures` 读取。Jev 运行需移除 `--allow-lab-fixtures`，另行显式增加 `--allow-model-api`；先完成 `REAL_ACCEPTANCE_PLAN.md` 的授权条件。

若崩溃，先确认发布者已停止，并保持核心/配置/所有权不变，使用原会话文件执行一次空规则恢复：

```powershell
./route-agent.exe recover-apply --config ./private/agent.json --allow-lab-fixtures --allow-owned-recovery --exclusive-controller --ownership ./private/session-ownership.json --state-file ./private/session-state.json
```

恢复只清空，不重启学习。校验失败、所有权过期或核心已变更时停止处理，不删除锁绕过检查，不重复尝试；保留诊断和会话文件。重新运行需新的所有权/状态路径。详细行为见 `RUNBOOK.md`，验收边界见 `ACCEPTANCE.md`。
