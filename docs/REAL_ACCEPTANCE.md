# 单次云端真实证据编排：待授权

2026-10-09：已按授权在 main 添加窄手动 launcher，固定执行旧受审代码 `08ec5ae340d06838f98bea7eca61c73ae921adb2`，并配置 main-only、用户审批、禁止管理员绕过的 environment。唯一真实运行 `37867539248` 已失败；本 PR 随后的修改仅改进诊断，未更新 main pin、读取 Secrets 或再次 dispatch。本地 VM 继续暂停。普通 CI 只执行 mock/loopback 测试。

当前累计模型预算与用户确认的专用 key 限额均为 USD 1。上次实际消耗没有证据，不能把重试当成新获 USD 1；不得自动重置额度。新运行前需要用户在服务商 UI 确认可用剩余额度，并确认下一次运行上限不超过剩余预算。这里不读取 key 值、不调用余额 API。

## 旧运行与新诊断的界限

旧运行通过源码校验、原生身份、物理路径和公开 core/worker 准备，在 execute 步骤仅返回 `refused_or_failed` 与退出码 1。根因、模型实际尝试数、费用及私有配置清理均未知，见 `evidence/real-acceptance-failure-2026-10-09.json`。固定版本的 mock 重演没有发现可确定解释该失败的路径或参数错误；不得根据 Secret 内容猜测，也不能用新测试倒填旧证据。

新版本输出固定 `diagnostic.stage/reason`；schema 只列六个已知字段名和固定类型标签，不显示字段值或未知键名。保留 core 启动退出码、worker 退出码；worker 的合法 JSON 即使非零退出也保留 DNS/TLS/VLESS/model 部分结果与 HTTP 尝试记录。请求 ID 可能包含服务端回显或敏感值，因此只输出 `present/absent/unknown`，绝不披露 ID、长度、响应正文或 header 列表。

worker 启动尝试之前计数为 `0/not_started`；一旦尝试启动，立即设为 `null/unknown`，直到收到严格白名单验证通过的完整 worker 结果。HTTP 计数在传输边界同步，失败也消耗尝试预算；它不是可收费请求数或账单。未收到 HTTP 响应时 request-ID 状态为 unknown。

Python finally 分别尝试回收 worker、core、删除本次私有目录、核对本次监听端口，并报告 `verified/failed/unknown/not_started`。一项清理失败不阻止其余步骤，也不覆盖原始失败阶段。spawn 被打断而未取得进程句柄时不能证明没启动，必须记 unknown；不枚举或杀其他 PID。硬终止/runner 崩溃可能没有 finally 记录，缺失即未知，不等于成功。

下一次最小审批仅涉及：将 main launcher 的固定 SHA 更新为本诊断补丁经 CI/独立审查后的精确版本；以用户确认的剩余累计额度上限运行一次原范围测试；仍最多两次模型尝试、20 分钟、用户亲自环境审批，不开启 TUN、不发布学习路由、不重试失败 run。当前“准备重试”不授权这些未来动作。


## 实际执行边界

受审 main 手动 launcher 固定 checkout 一个完整 40 位 SHA，其字面值同时是唯一允许 SHA。用户输入不能选择任意代码。PR 分支代码不能自行取得 environment secrets。后续修改执行代码必须重新审查、重新固定 SHA。只接受仓库本身、GitHub hosted Windows、main 的 `workflow_dispatch`、`run_attempt=1` 和指定工作流路径。独立 run 内有一次性启动标记；新建 dispatch 是新 run，仍需重新批准，不是系统层面的永久一次限制。

20 分钟 job 内先做无 secret 的依赖/build、原生身份与未放宽的物理路径检查、官方 core 下载及 archive/exe 双 SHA256 校验。真实步骤若已耗时超过 10 分钟直接拒绝。真实 worker 最长 4 分钟、父进程等待上限 270 秒，预留清理时间。可取消或 runner 崩溃时 Python finally 无法保证执行，最终依赖 GitHub 临时 VM 销毁；因此不要迁移到 self-hosted runner。正常与可捕获失败路径均终止本次进程并删除本次私有配置，不枚举/终止其他进程。

目标依次为 `example.com:443`、`www.cloudflare.com:443`：

1. 用生产 TLSCollector，经 `1.1.1.1:53` UDP（截断则 TCP）查询 A/AAAA。保留同一次解析首个地址，验证 Windows 物理路径和专属 core PID/启动时间。TLS 证书正常校验，只握手不发目标 HTTP 请求。
2. 无论直连是否成功，单独通过 loopback CONNECT、固定单节点 VLESS Reality TCP Vision 验证同一 IP 的 TLS。连接保持打开，用 core `/connections` 的来源端口、目标 IP:443、单一 `acceptance-vless` chain 核验关联；记录为 `vless_tls`，不改写生产直连证据。
3. 仅证据充分且显式 VLESS 验证成功时调用现有 NewJev，再交给现有 Accept。最多每主机一次、总计两次实际 POST，失败也计数，无额外“可用性测试”或重试。第一次请求同时验证服务是否可用。模型固定 `typesafe/jev-1.13`，端点固定 `https://openrouter.ai/api/alpha/decisions`；模型域名也由 1.1.1.1 解析并验证目标物理路径，TLS 不关闭验证。
4. 输出仅允许枚举、索引、计数。`model=answered, choice=UNCERTAIN` 是如实回答；`accepted=UNCERTAIN` 也可能是生产证据否决模型。DIRECT 成功不代表 PROXY 学习通过。此编排不写 provider、不发布学习规则，始终 `routing_updated=false, proxy_learning_tested=false, tun_tested=false`。

`status=completed` 只表示两主机显式 VLESS TLS 和模型回答均完成，不保证答案为 PROXY；DNS/传输/模型未完成则为 `partial`，即使工作流成功也不能宣称全项验收通过。guard/解析/进程/清理错误返回失败且只输出固定脱敏状态。

专属 standalone core 只监听 127.0.0.1，唯一规则为 MATCH 到唯一节点；TUN、DNS listener、sniffer、LAN、provider、geo 更新和 profile 缓存关闭。不操作用户 FlClash、系统 DNS/代理、网卡、路由或防火墙。此轮不重新覆盖历史真实 TUN 验收。

## 用户安全填写 secrets（只在后续授权后）

1. 在服务商网页新建专用 OpenRouter key，设置明确的账户侧支出限额和足够短的有效期。记录金额，不在聊天粘贴 key。确认该模型/端点可用于此 key；这里不会额外调用账户 API。最多两次请求不等于金额封顶，脚本的 `provider_cap_confirmed` 是用户确认，不是服务端验证。
2. 打开仓库 Settings → Environments → `real-acceptance-manual-approval`。该 environment 已按之前授权创建，无需重建；下一次运行前复核现有保护规则。设置仅 main 可部署、required reviewer，并检查管理员绕过配置。只写 environment 名称不会自动得到审批保护，缺失时 GitHub 可能创建无规则环境；激活前必须人工核实。若仅一个维护者，明确记录是否允许本人审批，不能配置不可满足的审批后声称可运行。
3. 在该 environment 的 Secrets 界面添加 `OPENROUTER_API_KEY`，值为专用 key。不要使用 repository-wide secret，不提交 `.env`，不从现有本机 dotenv 自动迁移。
4. 按 `examples/vless-node.template.json` 在可信本地编辑器填写一个节点，再将完整 JSON 粘贴到 environment secret `VLESS_NODE_JSON`。模板全为 null，故意不可运行。只允许 `server, port, uuid, servername, reality_public_key, short_id` 六字段；参考 schema 和运行时校验。server 必须是规范的公网数值 IP（不接受节点域名）；port 为整数；UUID 为小写规范格式；SNI 为规范公网 DNS 名；public key 为 32 字节规范 base64url；short id 为 0–8 字节小写十六进制。不要粘贴订阅链接、完整 profile、provider、规则或多个节点。此受限实现不兼容其他传输协议；不应修改 guard 来让现有节点勉强通过。
5. Secret JSON 是结构化敏感内容，GitHub 自动遮罩未必覆盖拆分字段；本实现依靠禁止原始 stdout/stderr/traceback、白名单结果和禁止 artifact/cache 上传。不要开启 Actions debug、打印环境、保存 core 日志或将私有配置加入工件。core 不接收模型 key，worker 不接收 VLESS JSON，控制器 secret 每次临时生成。
6. 授权且 launcher 已单独进入 main 后，在 Actions 手动运行，填写批准记录、正数 USD 上限、实际设置的 key 限额（不得超过批准金额；脚本额外限制 <=10 USD）、限额确认和节点费用确认。必填值缺失、0、NaN、超限均拒绝。每次点击运行、重跑或改 SHA 都须新的明确授权；本 launcher 拒绝 Actions rerun。
7. 审阅固定枚举结果与清理状态，运行后撤销专用 key、删除临时 environment secrets。撤销/删除由用户界面完成，不在聊天传输值。

## 最终一次审批必须明确的内容

在完整代码 SHA 和离线 CI 审查完成后，一次确认以下具体范围，不能将本文当成授权：

- main **只新增**受审 `.github/workflows/real-acceptance.yml` launcher，固定已审代码 SHA，不合并 PR 的产品代码；启用条件严格为指定仓库、main、手动且 attempt=1。
- 创建/设置上述 environment 的 main-only 部署规则、审批者和 bypass 策略；用户本人经 GitHub Secrets UI 填值，不授权代理读取本机真实凭据。
- 仅一次 GitHub standard Windows hosted 20 分钟 job；官方 core 固定 v1.19.32 与两项硬编码哈希；无 TUN、无宿主修改、无发布路由。
- 允许上述 DNS、两域名 TLS、用户选定节点 IP:port/SNI、OpenRouter 单端点和最多两次模型请求。准备阶段还需要 GitHub release/Go 依赖下载，均无 secrets。
- 明确模型最大 USD 金额、账户侧专用 key 限额，以及代理节点流量/费用许可。下一次可用剩余额度尚未确认；不能以“免费测试”假定收费服务成本为零。

诊断补丁的 main pin 更新和下一次执行尚未授权；当前不执行 prepare/execute、不改变保护环境或 main、不重置服务商 key 限额。
