# 本地模型历史评估（默认不启用）

**截至 2026-10-07，默认后端候选已改为 Jev API。** 本文保留 10-06 本机离线测量，供资源比较；不是当前部署要求。不重新下载第二个模型，不常驻 Python/CUDA。

| 项目 | 固定快照 |
|---|---|
| runtime | NandhaKishorM/laya@a4a8921afebfd852bba0000475cfb6ab737a124c，0.3.28 |
| checkpoint | convaiinnovations/laya-typed-decisions@e929ae5cf69bc34259cd2f95c9e91145b818b1f0 |
| 参数 / 权重 | 421,293,827；842,609,220 bytes |
| 权重 SHA256 | 4fa56de72383a9d3efa9cfa78955733c81b9fc8067a587ca4beb82c78107a24e |
| 软件 | Windows Python 3.12.13；torch 2.11.0+cu128；transformers 5.18.0 |
| GPU / CPU | eager bf16 CUDA / eager FP32 4 intra-op threads |

100 次 warm、5 次预热、4 客户端串行 worker burst；短英文 state 约 143 tokens。HF_HUB_OFFLINE / TRANSFORMERS_OFFLINE 开启，无公网推理 API。CUDA 每次同步，RSS 每 50 ms 采样。

| 指标 | GPU | CPU |
|---|---:|---:|
| import | 30.00 s | 12.74 s |
| 模型加载（import 后） | 21.82 s | 14.08 s |
| 首次推理 | 658.8 ms | 820.6 ms |
| warm P50 / P95 | 45.71 / 52.31 ms | 672.47 / 720.30 ms |
| warm 最大 | 59.94 ms | 768.97 ms |
| 四请求排队最后完成 | 182.5 ms | 2461.4 ms |
| 测量结束 RSS | 1807.2 MiB | 2274.8 MiB |
| 采样 RSS 峰值 | 3149.5 MiB | 3013.3 MiB |
| CUDA allocator peak reserved | 2520 MiB | 无 |

CUDA allocator 不是整机 GPU 占用；未测利用率/功耗，RSS 采样可能漏短峰值。import/load 各单次观测。权重约 0.785 GiB，原实验 venv 约 4.28 GiB；移除研究缓存后的部署磁盘只能估算约 5–6 GiB。

纯域名无证据合成输入的 100 次 raw argmax 均 DIRECT，初次 GPU DIRECT 0.4006 / PROXY 0.2813 / UNCERTAIN 0.3181。不能采纳最高概率或低 PROXY 当 DIRECT。与 Jev 使用的提示和状态不同，**不是匹配输入的公平准确率对比**。

[原始性能摘要](evidence/laya-summary-2026-10-06.json)。benchmark_laya.py 保留为可选历史实验，需要自行准备固定 checkpoint 到 .research/model、上述依赖及 psutil；默认 API 复现不执行它。

许可证在 10-06 核对：Laya runtime/checkpoint/ModernBERT 为 Apache-2.0；PyTorch 使用其 BSD-style 条款；Transformers Apache-2.0。仓库不分发权重/runtime。实际复用以固定资产文本为准：[Laya LICENSE](https://github.com/NandhaKishorM/laya/blob/a4a8921afebfd852bba0000475cfb6ab737a124c/LICENSE)、[checkpoint](https://huggingface.co/convaiinnovations/laya-typed-decisions/tree/e929ae5cf69bc34259cd2f95c9e91145b818b1f0)、[ModernBERT](https://huggingface.co/answerdotai/ModernBERT-large)。
