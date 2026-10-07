# Jev API 验证与后端选择

2026-10-07 本机实际调用官方 OpenRouter Decisions API。不是读取宣传延迟，不是生产 Agent 验收。

## 接口与方法

- endpoint：https://openrouter.ai/api/alpha/decisions
- 请求 model：typesafe/jev-1.13；实际响应：typesafe/jev-1.13-20260917。
- 输入 state + 单个 choice 问题，候选 DIRECT / PROXY / UNCERTAIN。
- 标准库 HTTP/1.1 client，明确进入本机 mixed-port，验证 TLS，不跟随重定向，忽略环境代理。
- 首请求计入 DNS/TCP/CONNECT/TLS；之后 40 次请求复用连接。4 并发各自新建 HTTPS 连接。
- 无自动重试，每个 socket timeout 8 s，用于观察，不是生产绝对 deadline。
- 12 个合成案例，每类 3 个：无证据、直连成功、直连失败+代理成功、证据冲突。数据明确标注 synthetic。
- Python 客户端版本和网络协议不能代替未来 Go HTTP/2 client 的表现；上线前再测。

官方参考：[请求/响应 schema](https://openrouter.ai/docs/api/api-reference/alphadecisions/submit-a-decisions-request)、[Jev 教程](https://openrouter.ai/blog/tutorials/how-to-use-jev/)。

## 测量结果

| 指标 | 当前观测 |
|---|---:|
| 首请求（主实验） | 1115.842 ms |
| 热顺序请求 | 40/40 成功 |
| 热 P50 / P95（nearest rank） | 476.340 / 538.866 ms |
| 热最大 | 652.717 ms |
| 新连接并发 4 请求 | 4/4 成功 |
| 并发 P50 / 最大 | 1085.262 / 1172.403 ms |
| 三态合成契约 | raw 与 policy 均 12/12 符合预期 |
| 主实验请求总数 | 57，均成功且有 usage.cost |
| 主实验上报费用 | $0.00113379 |
| 无显式 HTTP proxy 单请求 | 1612.202 ms；TUN 仍在，不是物理直连 |
| 最终隔离 Gate 的 Jev 请求 | 1045.521 ms；PROXY 0.99；决策次数 1 |

保留的实验结果共 60 次调用，另有第一次隔离成功后因事件标签修正复验的一次调用；本次所有调用上报费用约 $0.00121。这只是短输入和当前服务报价的观测，不是价格承诺。约 470 tokens 的单次成本约 $0.00002，生产需自行设预算，不复制这个数字作为固定配置。

[主实验完整结果](evidence/jev-api-2026-10-07.json)、[显式代理探测](evidence/jev-proxy-probe-2026-10-07.json)、[无显式代理探测](evidence/jev-no-http-proxy-2026-10-07.json)、[隔离闭环](evidence/gate-jev-2026-10-07.json)。

## 有效性与边界

40 个合成未知域名在缺少网络证据时均 raw UNCERTAIN。合成明确事实能得到正确分类，但这些事实也可以由确定性代码直接判断。**不能把 12/12 称为 100% 路由准确率，更不能据此证明模型增益。** 没有实际探测网站 TCP/TLS，也没有建立实时 GFW ground truth。

0/57 失败只是本次样本，不能推出 0% 长期失败率。尚未验证跨时段、节点切换、休眠恢复、服务故障、限流与长期运行。离线最小边界测试仅验证证据 veto、非法响应、proxy 503/redirect/timeout 不被采纳；不等同真实 OpenRouter 故障演练或整个 DNS hard deadline。

## 选择

优先候选 Jev HTTP 后端，Laya 不默认加载；只有纯 Go 控制器承担日常资源。建议正式默认 async，bounded-preflight 先 opt-in 实验，1.5 s 是初始预算，不是 SLA。原 300 ms 已不成立。实际 API→provider→DNS 验收证明约一秒等待的隔离机制可行，不能保证当前 FlClash/TUN 首连正确。

缺少有效证据时生产 policy 强制 UNCERTAIN；API 不可用不影响规则/缓存/DNS fallback。服务 endpoint/bootstrap、每日调用上限、队列/并发、熔断与 key 存储在实施阶段完成。不使用账号自动充值或自动改账单配置。
