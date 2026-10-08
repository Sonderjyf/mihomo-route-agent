# 单次云端真实证据编排：待授权

当前仅 PR 分支实现与离线验证。`examples/github-real-acceptance.yml.disabled` 在工作流目录外，job 也显式禁用。尚未改 main、创建 environment、上传密钥或运行真实 DNS/TLS/Mihomo/Jev；本地 VM 继续暂停。普通 CI 只执行 mock/loopback 测试。

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
2. 打开仓库 Settings → Environments → `real-acceptance-manual-approval`。创建/配置此 environment 本身也属于待批准动作。设置仅 main 可部署、required reviewer，并检查管理员绕过配置。只写 environment 名称不会自动得到审批保护，缺失时 GitHub 可能创建无规则环境；激活前必须人工核实。若仅一个维护者，明确记录是否允许本人审批，不能配置不可满足的审批后声称可运行。
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
- 明确模型最大 USD 金额、账户侧专用 key 限额，以及代理节点流量/费用许可。当前金额未批准；不能以“免费测试”假定收费服务成本为零。

没有获得以上权限前保持禁用，不执行 prepare/execute，不创建 environment，不改 main。
