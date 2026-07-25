# ModelPort OpenAI-Compatible Providers

This document is the capability contract for the eight dedicated
OpenAI-compatible provider presets added by ModelPort. DeepSeek is an existing
provider and is outside this eight-provider matrix.

All official sources listed here were accessed on **2026-07-25**. Capability
labels have these meanings:

- **Confirmed**: the cited official documentation describes the capability.
- **Model-dependent**: the official documentation makes support depend on the
  selected model, endpoint, or routed provider.
- **ModelPort adapter**: ModelPort converts the downstream protocol to upstream
  Chat Completions; this is not a claim of native upstream support.
- **Not confirmed**: the cited sources do not establish the capability. This
  does not mean the provider can never support it.
- **Disabled in current adapter**: ModelPort deliberately does not expose the
  operation for this preset, regardless of undocumented upstream behavior.

Registry model values are suggestions, not an allowlist or an official product
catalog. Operators may use any model or endpoint identifier accepted by their
upstream account.

## Identity, discovery, and authentication

| Provider | Internal ID | Default base URL | Authentication | Model list in ModelPort |
| --- | --- | --- | --- | --- |
| Alibaba Cloud Model Studio / Qwen | `qwen` | `https://dashscope.aliyuncs.com/compatible-mode/v1` | `Authorization: Bearer <key>` | Enabled: `GET /models` |
| Zhipu AI / GLM | `glm` | `https://open.bigmodel.cn/api/paas/v4` | `Authorization: Bearer <key>` | Enabled: `GET /models` |
| Moonshot AI / Kimi | `kimi` | `https://api.moonshot.cn/v1` | `Authorization: Bearer <key>` | Enabled: `GET /models` |
| ByteDance | `doubao` | `https://ark.cn-beijing.volces.com/api/v3` | `Authorization: Bearer <key>` | Disabled; enter an Ark model or Endpoint ID manually |
| SiliconFlow | `siliconflow` | `https://api.siliconflow.cn/v1` | `Authorization: Bearer <key>` | Enabled: `GET /models` |
| OpenRouter | `openrouter` | `https://openrouter.ai/api/v1` | `Authorization: Bearer <key>` | Enabled: `GET /models` |
| MiniMax | `minimax` | `https://api.minimaxi.com/v1` | `Authorization: Bearer <key>` | Disabled; use registry suggestions or enter a model manually |
| Xiaomi MiMo | `mimo` | `https://api.xiaomimimo.com/v1` | ModelPort uses `Authorization: Bearer <key>`; the official API also accepts `api-key` | Disabled; use registry suggestions or enter a model manually |

`doubao` is retained only as the database and API identifier for migration and
configuration compatibility. The external and user-facing provider name is
**ByteDance**.

Model synchronization is an implementation switch, not an assertion that a
disabled provider has no upstream discovery API. A failed sync does not replace
the saved model whitelist.

## Protocol and response matrix

| Provider | Native Chat Completions | Native Responses | Native Messages | ModelPort `/v1/responses` | ModelPort `/v1/messages` | Streaming | Usage |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Qwen | Confirmed | Not confirmed; not used | Not confirmed; not used | ModelPort adapter | ModelPort adapter | Confirmed Chat SSE; adapter streaming supported | Parses OpenAI-style Chat `usage` |
| GLM | Confirmed | Not confirmed; not used | Not confirmed; not used | ModelPort adapter | ModelPort adapter | Confirmed Chat SSE; adapter streaming supported | Parses OpenAI-style Chat `usage` |
| Kimi | Confirmed | Not confirmed; not used | Not confirmed; not used | ModelPort adapter | ModelPort adapter | Confirmed Chat SSE; adapter streaming supported | Parses OpenAI-style Chat `usage` |
| ByteDance | Confirmed | Not confirmed; not used | Not confirmed; not used | ModelPort adapter | ModelPort adapter | Confirmed Chat SSE; adapter streaming supported | Parses OpenAI-style Chat `usage` |
| SiliconFlow | Confirmed | Not confirmed; not used | Not confirmed; not used | ModelPort adapter | ModelPort adapter | Confirmed Chat SSE; adapter streaming supported | Parses OpenAI-style Chat `usage` |
| OpenRouter | Confirmed | Not confirmed for every routed provider; not used | Not confirmed; not used | ModelPort adapter | ModelPort adapter | Confirmed Chat SSE; adapter streaming supported | Parses OpenAI-style Chat `usage` |
| MiniMax | Confirmed | Not confirmed; not used | Not confirmed; not used | ModelPort adapter | ModelPort adapter | Confirmed Chat SSE; adapter streaming supported | Parses OpenAI-style Chat `usage` |
| Xiaomi MiMo | Confirmed | Not confirmed; not used | Not confirmed; not used | ModelPort adapter | ModelPort adapter | Confirmed Chat SSE; adapter streaming supported | Confirmed Chat `usage`; ModelPort parses it |

For all eight presets, ModelPort's upstream transport is
`POST /chat/completions`. It does not call an upstream Responses API, Messages
API, or Responses WebSocket. Responses WebSocket is unsupported by these
presets. ModelPort forces `stream_options.include_usage=true` on upstream Chat
streams for billing; an upstream may reject or omit fields it does not support.
First-token timing requires a streaming downstream request and is measured from
the first non-usage output chunk.

## Tools, vision, and JSON Schema

| Provider | Tool calling | Vision or multimodal input | JSON Schema / structured output |
| --- | --- | --- | --- |
| Qwen | Model-dependent | Model-dependent; use a compatible Qwen-VL model | Model-dependent; unsupported schemas remain upstream errors |
| GLM | Confirmed, with model-specific limits | Model-dependent | Not confirmed as a uniform capability across models |
| Kimi | Confirmed, with model-specific limits | Model-dependent | Not confirmed as a uniform capability across models |
| ByteDance | Model-dependent by Ark model or endpoint | Model-dependent by Ark model or endpoint | Model-dependent; no preset-wide guarantee |
| SiliconFlow | Model-dependent | Model-dependent | Model-dependent |
| OpenRouter | Model- and routed-provider-dependent | Model- and routed-provider-dependent | Model- and routed-provider-dependent |
| MiniMax | Confirmed for current text models | Confirmed for MiniMax-M3 image/video input; audio input is explicitly unsupported by the cited OpenAI-compatible page | Not confirmed by the cited OpenAI-compatible page |
| Xiaomi MiMo | Confirmed; `tool_choice` effectively supports `auto` | Model-dependent; the official page names the models accepting image/audio/video | Confirmed only for strict tool parameters, using a subset of JSON Schema; response-schema support is not confirmed |

ModelPort passes compatible message parts, tool fields, and `response_format`
through to Chat Completions, but it cannot add a feature absent from the chosen
upstream model. “Not confirmed” must not be presented to users as supported or
unsupported without a newer official source and a provider-specific test.

## Request IDs, errors, and parameters

| Provider | Upstream request ID headers recognized by ModelPort | Error handling | Provider-specific parameter behavior |
| --- | --- | --- | --- |
| Qwen | `x-request-id`, then `request-id` | Common extractor; provider-specific schema is not fully normalized | Request JSON is passed through; model-specific rejection remains upstream behavior |
| GLM | `x-request-id`, then `request-id` | Common extractor; provider-specific schema is not fully normalized | Request JSON is passed through; model-specific rejection remains upstream behavior |
| Kimi | `x-request-id`, then `request-id` | Common extractor; provider-specific schema is not fully normalized | Request JSON is passed through; model-specific rejection remains upstream behavior |
| ByteDance | `x-request-id`, `x-tt-logid`, then `request-id` | Common extractor; provider-specific schema is not fully normalized | Stored `endpoint_id`, when present, replaces the downstream `model` value |
| SiliconFlow | `x-request-id`, then `request-id` | Common extractor; provider-specific schema is not fully normalized | Request JSON is passed through; model-specific rejection remains upstream behavior |
| OpenRouter | `x-request-id`, then `request-id` | Common extractor; final error semantics can depend on the routed provider | Adds `HTTP-Referer: https://modelport.link` and `X-Title: ModelPort`; account overrides take precedence |
| MiniMax | `x-request-id`, `trace-id`, then `request-id` | Common extractor; provider-specific schema is not fully normalized | Drops `presence_penalty`, `frequency_penalty`, and `logit_bias`; accepts `n` only when it is `1` |
| Xiaomi MiMo | `x-request-id`, then `request-id` | Common extractor; official HTTP error codes are documented separately | In thinking mode some models override `temperature` and `top_p`; non-`auto` `tool_choice` is removed by the upstream service |

The common error extractor recognizes `error.message`, `error.detail`,
`error.msg`, `base_resp.status_msg`, `error_msg`, `msg`, `detail`, and top-level
`message`. ModelPort preserves the upstream HTTP status where its error policy
allows, but it does not promise lossless preservation of every provider's
error object. Provider-specific error bodies and request ID headers not listed
above are not normalized.

Except for the explicit MiniMax rules, ModelPort does not validate every
provider/model parameter combination. `stream_options`, `reasoning_effort`,
`response_format`, and `parallel_tool_calls` are passed through when present;
acceptance and semantics remain the responsibility of the upstream model.

## Pricing strategy

| Provider | Official price authority | ModelPort policy | No default claim |
| --- | --- | --- | --- |
| Qwen | Alibaba Cloud Model Studio pricing page | Use maintained model pricing when available; administrators may configure model/channel pricing | No numeric price is asserted here |
| GLM | Zhipu AI current model documentation or console | Use maintained model pricing when available; administrators may configure model/channel pricing | No numeric price is asserted here |
| Kimi | Kimi official pricing page | Use maintained model pricing when available; administrators may configure model/channel pricing | No numeric price is asserted here |
| ByteDance | Ark official model pricing page and the account's endpoint configuration | Configure the callable model/endpoint price explicitly when maintained data has no match | No Chat model default price is asserted here |
| SiliconFlow | SiliconFlow live model catalog | Use the selected catalog model's current price or an administrator override | No numeric price is asserted here |
| OpenRouter | OpenRouter live model catalog; price depends on model/provider route | Use maintained routed-model pricing or an administrator override | No provider-wide price exists |
| MiniMax | MiniMax official pricing page | Use maintained model pricing when available; administrators may configure model/channel pricing | No numeric price is asserted here |
| Xiaomi MiMo | Xiaomi MiMo pay-as-you-go pricing page | Use maintained model pricing when available; administrators may configure model/channel pricing | No numeric price is asserted here |

Prices change independently of ModelPort releases. This document intentionally
contains no copied numeric prices. Unknown models, aliases, Ark Endpoint IDs,
and OpenRouter routes require an administrator-verified billing configuration;
never infer a price from a similar model name.

## Explicit limitations by provider

- **Qwen:** no native Responses or Messages guarantee is made; tools, vision,
  and schema support must be checked for the selected model.
- **GLM:** no native Responses or Messages guarantee is made; vision and schema
  behavior is not uniform across models.
- **Kimi:** no native Responses or Messages guarantee is made; multimodal and
  schema behavior must be checked against the selected model documentation.
- **ByteDance:** model sync is disabled in the current adapter; an Ark
  `endpoint_id` can override `model`; no preset-wide multimodal or schema claim
  is made.
- **SiliconFlow:** capabilities and pricing follow the selected hosted model;
  the platform preset does not guarantee tools, vision, or schema support.
- **OpenRouter:** behavior, errors, limits, and pricing can change with the
  routed model/provider; ModelPort does not use an upstream Responses route.
- **MiniMax:** model sync is disabled; deprecated `function_call` is explicitly
  unsupported by the cited page, `n` only accepts `1`, three penalty/bias fields
  are removed, and the cited MiniMax-M3 route does not support audio input.
- **Xiaomi MiMo:** model sync is disabled; only a subset of JSON Schema is
  accepted for strict tools, `tool_choice` effectively supports only `auto`,
  and thinking-mode sampling overrides and multimodal support are model-specific.

## Official references

All references below were accessed on **2026-07-25**.

| Ref | Provider | Official source |
| --- | --- | --- |
| Q1 | Qwen | [OpenAI compatibility](https://help.aliyun.com/zh/model-studio/compatibility-of-openai-with-dashscope) |
| Q2 | Qwen | [Model pricing](https://help.aliyun.com/zh/model-studio/model-pricing) |
| G1 | GLM | [OpenAI SDK compatibility](https://docs.bigmodel.cn/cn/guide/develop/openai/introduction) |
| K1 | Kimi | [API overview](https://platform.kimi.com/docs/overview) |
| K2 | Kimi | [Pricing](https://platform.kimi.com/docs/pricing) |
| B1 | ByteDance | [Ark OpenAI compatibility](https://docs.volcengine.com/docs/82379/1330626?lang=zh) |
| B2 | ByteDance | [Ark model pricing](https://docs.volcengine.com/docs/82379/1099320) |
| S1 | SiliconFlow | [Chat Completions](https://docs.siliconflow.cn/cn/api-reference/chat-completions/chat-completions) |
| S2 | SiliconFlow | [Live model catalog](https://cloud.siliconflow.cn/models) |
| O1 | OpenRouter | [API overview](https://openrouter.ai/docs/api_reference/overview.md) |
| O2 | OpenRouter | [Live model catalog](https://openrouter.ai/models) |
| M1 | MiniMax | [OpenAI-compatible text API](https://platform.minimax.io/docs/api-reference/text-openai-api) |
| M2 | MiniMax | [Pricing](https://platform.minimax.io/docs/pricing) |
| X1 | Xiaomi MiMo | [Documentation index](https://platform.xiaomimimo.com/llms.txt) |
| X2 | Xiaomi MiMo | [OpenAI-compatible Chat API](https://platform.xiaomimimo.com/static/docs/api/chat/openai-api.md) |
| X3 | Xiaomi MiMo | [Models](https://platform.xiaomimimo.com/static/docs/quick-start/model.md) |
| X4 | Xiaomi MiMo | [Hyperparameters](https://platform.xiaomimimo.com/static/docs/quick-start/model-hyperparameters.md) |
| X5 | Xiaomi MiMo | [Error codes](https://platform.xiaomimimo.com/static/docs/quick-start/error-codes.md) |
| X6 | Xiaomi MiMo | [Pay-as-you-go pricing](https://platform.xiaomimimo.com/static/docs/price/pay-as-you-go.md) |
