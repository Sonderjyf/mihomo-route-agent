# Third-party references

Original scripts and documentation in this repository are MIT licensed. Referenced projects retain their own licenses. This repository does not vendor their source, model weights, Windows drivers, or executables.

| Project / package | Use in this repository | Upstream license noted at review |
|---|---|---|
| Go | Native Agent toolchain | BSD-3-Clause |
| miekg/dns v1.1.68 | UDP/TCP DNS library | BSD-3-Clause |
| go.yaml.in/yaml/v4 v4.0.0-rc.3 | Offline profile YAML parsing; pinned release candidate | Apache-2.0 |
| golang.org/x/net, x/sync and indirect x/* | IDNA/PSL, singleflight, transitive tooling | BSD-3-Clause |
| Python | Manual experiment interpreter | PSF |
| dnspython 2.8.0 | Isolated DNS smoke dependency | ISC |
| PyYAML 6.0.3 | Isolated configuration dependency | MIT |
| FlClash / Mihomo | External client / separately downloaded core | GPLv3 |
| FlClash-rules / rule-inspector / smart-selector | Architecture reference only | MIT |
| mosdns / Portmaster | Architecture reference only | GPL family, inspect pinned asset |
| WinDivert | Deferred architecture reference only | LGPLv3 or GPLv2 |
| Laya runtime / checkpoint / ModernBERT | Optional historical local benchmark | Apache-2.0 |
| PyTorch / Transformers / psutil | Optional historical benchmark dependencies | Their respective upstream licenses |
| TypeSafe Router | Design reference only; no code copied | No explicit license found at snapshot |

See SOURCE_REVIEW.md and LOCAL_MODEL.md for pinned sources and licenses. Jev is accessed as a hosted API subject to the actual service/account terms; an API subscription is not an open-source model license. No third-party snippet or entire implementation was copied into the experiment client.
