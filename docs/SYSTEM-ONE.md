# 决策配置（System One）

在侧栏打开 **决策配置**，创建配置，测试接口，再启用 MCP 服务。启用后会生成 `systemone_<配置ID>` 虚拟服务和 `evaluate` 工具；在服务管理中可将它加入分组，Agent 通过 New MCP 的已有 API Key 和 MCP 地址调用。配置页面不会回显上游 API 密钥。

## 接口配置

| 接口类型 | 地址填写方式 | 默认地址 | 默认模型 |
| --- | --- | --- | --- |
| TypeSafe 兼容 | 服务根地址；自动追加 `/v1/systemone` | `https://api.typesafe.ai` | `jev-latest` |
| OpenRouter Decisions | 完整接口地址 | `https://openrouter.ai/api/alpha/decisions` | `~typesafe/jev-latest` |

TypeSafe 官方 Jev 使用 TypeSafe 兼容类型。自行部署的 CLM、Laya 只要实现 `POST /v1/systemone`，也用这一类型；例如 CLM 可填写 `http://127.0.0.1:8700` 和 `clm-latest`。`127.0.0.1` 指运行 New MCP 后端的机器或容器。无鉴权的本地服务仍需填非空密钥（如 `local`），请求会以 Bearer token 发送。Laya 模型名由部署端决定。

OpenRouter 地址是完整的 Decisions 接口路径。工具调用可通过 `model` 参数覆盖配置的默认模型。测试按钮会发出一次真实的判断请求，可能产生上游费用。

## `evaluate` 工具

以下 JSON 是 `evaluate` 工具的 `arguments` 参数对象，不是完整的 MCP JSON-RPC 请求。启用配置并将生成的服务加入分组后，直连模式使用工具名 `systemone_<配置ID>__evaluate` 调用 `tools/call`；智能模式使用工具 ID `systemone_<配置ID>.evaluate` 调用 `mcp.execute`：

```json
{
  "state": {"message": "My payouts have failed for 3 days"},
  "questions": {
    "urgent": {"type": "noul", "instructions": "Does the message express urgency?"},
    "team": {
      "type": "choice",
      "instructions": "Which team should handle this?",
      "criteria": {"billing": "Payments", "technical": "Bugs", "other": "No match"},
      "min_confidence": 0.7
    },
    "impact": {
      "type": "score",
      "instructions": "How severe is the impact described by the customer?",
      "criteria": ["Low impact", "Moderate impact", "High impact"]
    }
  }
}
```

`noul` 给出条件成立的概率；`choice` 给出选项及各选项概率；`score` 接受由低到高的等级数组，返回概率加权分数和等级分布。低于 `min_confidence` 的 `noul`/`choice` 回答会标记 `uncertain`，`choice` 会变成 `__uncertain__`，原概率仍保留。置信度衡量概率分布的集中程度，不能保证判断正确。

`items` 可传最多 500 条独立记录，同一组问题逐条评估。每条记录都是单独的上游请求，最多同时发送 8 条；成功项进入 `results`，失败项进入 `errors`。批量调用可能产生多笔上游费用。单次响应与汇总结果上限为 16 MiB；上游调用超时 60 秒，对 429/529 最多重试 3 次。

这个实现改编自仓库的 `system-one-connector`（MIT License）：沿用其问题校验、批量处理、概率排序和低置信度规则，由 New MCP 管理 HTTP MCP 网关、配置和服务生命周期。
