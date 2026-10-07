# 限制与未证明事项

## 当前缺口

- Jev 性能和合成证据契约已验，不是未知域名的实时路由准确率。云端也不知道当前本机/节点是否可达；没有证据时仍 UNCERTAIN。
- 现用 FlClash 有 610 条规则但未注入 Gate/provider；订阅更新、matcher 对齐和 API bootstrap 尚未验收。
- bundled core provider 写入未测；独立官方 core 成功不替代此验证。
- 浏览器有效 DoH、整机 DNS 入口、TUN IP→hostname 关联未完成。
- API 新连接可超过一秒；300 ms 目标已失败，1.5 s 新预算是实验参数。
- Go 产品尚未开发，30–100 MiB 是预算；暂无服务恢复、长期运行与资源实测。
- 真实 API 的失败/限流/跨时段抖动未模拟；离线 socket 测试不是整个 DNS hard deadline。
- 输入的域名/必要事实会离开本机，并产生费用；密钥只在本地，公开证据不含访问历史。上线前核对实际账号/供应商数据设置，不假设零保留。

## 覆盖与一致性

DNS Gate 只覆盖首次连接前确实可观察的 query。应用 DoH/DoQ、hosts、已有 DNS 缓存、IP literal、mDNS、远端域名解析及已建立连接可绕开。系统代理 hostname 与直接 TUN IP 是不同入口。

普通 Fake-IP 会跳过上游 Gate。redir-host 的共享 IP/CNAME/HTTPS-SVCB hints/映射过期可能丢失身份；ECH 限制后续 sniffing，不等于所有普通 DNS 不可见。QTYPE 不可互相替代。

/connections 是事后审计，短连接可能采不到；新规则不迁移已有会话。async 只影响未来新连接；bounded-preflight 只在期限内成功时覆盖。fallback 不是 learned DIRECT。

四 provider 无统一事务。DB TTL 清理需同步移除 provider；core/profile/规则版本变化需重建 applied 状态。DNS 挂起不能阻止另一个有缓存 IP 的进程连接。

模型/API 死亡与 Gate 死亡不同；Gate 进程退出后内部 fallback 无效，需要独立恢复/旁路。不能加竞速 nameserver 同时声称严格等待与自动旁路。

## Deferred

WFP/WinDivert、100% Windows 连接覆盖、自研 TUN、FlClash/Mihomo fork、Hybrid Fake-IP、复杂 Dashboard/Sub-Store、自动训练、复杂主动 probe、页面 GET、跨设备同步、移动端和 Linux 产品化。

Laya/其他本地模型仅保留可选历史实验。OpenRouter 生产依赖从“禁止项”改为明确可选后端，受 deadline、预算、熔断和 fallback 约束；尚未实际启用。
