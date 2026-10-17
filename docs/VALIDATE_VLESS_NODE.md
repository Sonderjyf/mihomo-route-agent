# 本地检查 VLESS_NODE_JSON

在本地运行 `scripts/validate_vless_node.py`，复用真实验收的 `node_input`。仅检查格式，不测试节点可达性，不联网、不启动 core、不读取剪贴板或环境中的凭据，也不保存输入。无需安装 Python 第三方依赖。

## Windows 最简用法

在包含这两个文件的仓库目录打开 PowerShell：`scripts/validate_vless_node.py` 和 `scripts/real_acceptance.py`。使用已经安装的 Python 3：

```powershell
py -3 -B -X utf8 scripts\validate_vless_node.py --file "C:\你的私密目录\node.json"
```

没有 `py` 命令但已有 `python` 时，将 `py -3` 替换为 `python`。路径指向你本人准备的 UTF-8 JSON 文件；工具只读该文件，不创建副本。将文件放在仓库外，不要提交。也可通过 stdin 提供 UTF-8 JSON，省略 `--file`；不要将节点原文写进命令行或 shell 历史。

输出是单行安全 JSON。`valid=true`、退出码 0 表示通过格式校验；退出码 1 表示失败。失败时只输出固定 `error`、从 1 开始的 JSON `line/column`（无可用位置时为 null）、允许的字段名及 `expected_type`，不包含原文、值、路径、任意异常消息或上下文片段。

行列号来自当前 Python JSON 解析器；不同 Python 版本可能将同一语法错误定位到相邻位置。行列号指向本地文件中的原文，工具不会修改中文引号、去除围栏/BOM、解开转义或自动修复内容，保证与真实入口相同的接受条件。

## 准备输入

从 [六字段模板](../examples/vless-node.template.json) 填写一个 JSON 对象。模板中的 null 故意不可用。端口为整数，其余五个字段为字符串；只允许 `server, port, uuid, servername, reality_public_key, short_id`，具体约束见 [schema](../examples/vless-node.schema.json)，以运行时校验为准。

使用英文双引号，不带 Markdown 围栏、注释、末尾逗号，不粘贴 VLESS URI、YAML 或订阅内容。`short_id` 为字符串，节点确实使用空 ID 时才填 `""`。公网数值 IP、TCP/REALITY/Vision 等受限条件保持不变。

通过后，由你本人将相同 JSON 对象内容写入 GitHub environment 的 `VLESS_NODE_JSON`，不要额外包一层引号或将换行替换成字面的 `\n`。本地通过仅证明该本地文本可解析，不能证明 GitHub 中存储的内容相同，也不会启动或授权下一次真实运行。

## 环境变量链路只读核对（2026-10-09）

检查对象：main launcher 提交 `17ab9efc7cdc3e982547be930e26fae856d3ca2f`，固定执行 `6870f4de61d8f2b9ffb2fe21b3f94a8516a4b71e`。

- workflow 的 secret 步骤直接设置 `VLESS_NODE_JSON: ${{ secrets.VLESS_NODE_JSON }}`，名称与 Python `env.get("VLESS_NODE_JSON", "")` 一致。
- `run` 仅为 `python -B scripts/real_acceptance.py execute`，没有把 JSON 插进 PowerShell 源码，没有 shell 引号拼接、base64、JSON 二次编码或反转义。
- Python 使用 `env = dict(os.environ)`，随后原样传给 `node_input`；该路径不做 strip、替换或文本修复。`PYTHONUTF8=1` 也不负责 JSON 转义。
- 普通离线测试以合成 CRLF 多行 JSON 验证子进程环境变量到同一校验函数的路径；这不等于检查了 GitHub 的真实 Secret。

在已检查代码中没有找到额外转义或名称不一致。实际 Secret 内容未读取，因此不能确定第三次 `invalid_json` 的具体原因，也不能断言一定是用户输入错误。缺失/空值同样可能进入 invalid_json；日志遮罩破坏输出 JSON 标点本身不证明输入被 runner 改写。

当前工具变更只在 draft PR；不更新 main 固定 SHA、不配置 Secret、不查询费用、不执行真实验收。
