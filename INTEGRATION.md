# FlClash / Mihomo 集成

本次只做隔离实验与只读刷新，未注入现用 FlClash。610 条规则不等于 Agent 已拥有一致 UNKNOWN 定义；官方实验 core 与 bundled core 必须分别验收。

## 注入与所有权

订阅关联 JavaScript overwrite，入口 function main(config)：保留节点来源，生成受控组/规则栈。script 后仍有 FlClash 字段补丁，DNS override、appendSystemDns、TUN/controller 可能改变结果；检查最终预览和活动 API。

用 HTTP localhost Rule Provider，避免依赖 profile 私有文件路径；path 可能被重写。Agent matcher 与 core 共用规则清单。learned 只写精确 DOMAIN，不扩大成 DOMAIN-SUFFIX。

参考：[FlClash v0.8.99](https://github.com/chen08209/FlClash/tree/v0.8.99)、[FlClash-rules 固定源码](https://github.com/liuzq1103/FlClash-rules/tree/73d152075c567f3d8b0c853d1bccc9d5910716dd)。不是可直接复制的完整节点专用方案。

## API bootstrap

- 官方 API 域名设固定显式出站，不进入 Jev 判定；不要泛化到共享 CDN。
- Agent HTTPS 显式使用指定回环 mixed-port，固定目标、TLS 校验、禁止重定向；key 不交给 provider/Controller。
- benchmark 成功只证明现有路径；正式组不能依赖正等待的未知域名判断。
- API/节点 bootstrap DNS 独立，不能回 Gate 或未经核实的 system resolver。
- Controller/provider 使用回环 IP，禁用环境代理。

受控 profile 中采实际 API connection chain，再验启动、节点故障、模型断连。显式 HTTP 代理入口不是物理出站证明。

## 未启用的配置轮廓

Gate 产品尚未实现，以下不是可直接替换订阅的配置：

~~~yaml
dns:
  enable: true
  enhanced-mode: redir-host
  nameserver:
    - udp://127.0.0.1:15355
rule-providers:
  learned-proxy:
    type: http
    behavior: classical
    format: yaml
    url: http://127.0.0.1:18765/rules/learned-proxy.yaml
    path: ./route-agent/learned-proxy.yaml
    interval: 3600
~~~

其余为 manual-direct/manual-proxy/learned-direct。provider 内容：

~~~yaml
payload:
  - DOMAIN,api.example.com
~~~

普通 Fake-IP 跳过上游 Gate，首轮 redir-host。Gate upstream 不能回 Mihomo/system://；并列不受控 nameserver 可能绕过等待。Gate 看见 query 不等于 TUN IP 连接能恢复同一 hostname。

## 提交与回滚

完整 snapshot → PUT /providers/rules/{name} → 204 → GET /providers/rules 核对元数据 → applied。同步之后放行 DNS；异步只影响未来连接。不用整体 configs reload 更新一个域名。

ACK、条数、更新时间、内容 hash 是不同证据。审计 rule=RuleSet、rulePayload=provider 名称和实际 chains。跨 provider 无统一事务，部分失败不标 applied、恢复上次有效 snapshot。

本轮不执行系统切换。实施时本地保存原 profile/脚本/DNS，建独立实验 profile；验最终规则、分流 DNS/虚拟网络、浏览器有效 DoH。先显式 DNS+hostname SOCKS，再直接 socket/TUN IP，分别报告；连接维持到审计采到。验证订阅刷新、core 重启、API 失败和 Gate 死亡旁路后才扩大使用。

回滚使用 FlClash 正常切回原 profile/解除实验覆写并重新应用，恢复保存的 DNS 选项；核对正常网络和内部名称解析，不直接编辑生成 YAML。
