# selection-assist-api Specification

## ADDED Requirements

### Requirement: Assist 任务端点

系统 SHALL 提供 `POST /api/ai/assist/<task>` 端点（task ∈ `translate` | `polish` | `review`），接收 JSON body（`selected_text` 必填；`context_before`/`context_after` 可选；`full_text` 对 review 必填），以 SSE 流式返回 `start`、`delta`、`done`、`error` 四类事件，模型调用 MUST 复用 `AIService` 的全局配置。

#### Scenario: 翻译任务流式返回
- **WHEN** 已登录用户以 `{ "selected_text": "选区文本", "context_before": "...", "context_after": "..." }` 调用 `POST /api/ai/assist/translate`
- **THEN** 响应为 `text/event-stream`，依次发出 `start`（含任务类型）、若干 `delta`（增量文本）、`done`（规整后结果）
- **AND** `done` 载荷包含 `translation` 字符串与 `hard_words` 数组（每项含 `word`、`gloss`）

#### Scenario: 润色任务返回三个候选
- **WHEN** 用户调用 `POST /api/ai/assist/polish` 并携带有效选区
- **THEN** `done` 载荷包含 `candidates` 数组，恰好 3 项，每项含 `style`（`formal`/`concise`/`restructure`）与 `text`

#### Scenario: 评价任务要求全文上下文
- **WHEN** 用户调用 `POST /api/ai/assist/review` 且未提供 `full_text`
- **THEN** 响应为 400 错误，不发起 LLM 调用

#### Scenario: 非法任务名
- **WHEN** 用户调用 `POST /api/ai/assist/summarize`（不在支持列表内）
- **THEN** 响应为 404

### Requirement: LLM 输出规整

服务端 SHALL 对模型原始输出做规整后放入 `done` 事件：剥离 code fence、提取首个 JSON 对象、按任务校验必需字段；规整失败时 SHALL 发出 `error` 事件并结束流，不得把未规整文本伪装成成功结果。

#### Scenario: 模型输出被 code fence 包裹
- **WHEN** 模型输出为 ```json ... ``` 包裹的合法 JSON
- **THEN** `done` 载荷为解析后的结构化对象

#### Scenario: 评价无问题
- **WHEN** 评价任务返回 `{"issues": []}`
- **THEN** 流正常以 `done` 结束，`result.issues` 为空数组

#### Scenario: 模型输出夹带说明文字
- **WHEN** 模型输出形如“以下是结果：{...}”
- **THEN** 服务端提取其中的 JSON 对象并正常返回 `done`

#### Scenario: 输出无法解析
- **WHEN** 模型输出经规整后仍不是合法且字段齐全的 JSON
- **THEN** 流以 `error` 事件结束，错误信息指明解析失败

### Requirement: 输入上限与鉴权

assist 端点 MUST 处于全局认证守卫之后（未登录返回 401），并 SHALL 强制输入上限：`selected_text` ≤ 5000 字符、`context_before`/`context_after` 各 ≤ 2000 字符、`full_text` ≤ 30000 字符；超限返回 400 且不发起 LLM 调用。review 的 `full_text` 超限时 MUST 整体拒绝而非截断。

#### Scenario: 未登录调用
- **WHEN** 未携带有效会话调用任一 assist 端点
- **THEN** 响应为 401

#### Scenario: 选区超长
- **WHEN** `selected_text` 长度为 6000 字符
- **THEN** 响应为 400，错误信息说明上限

### Requirement: 上游错误语义

上游 LLM 调用失败或 AI 服务未配置时，系统 SHALL 以 SSE `error` 事件（调用中失败）或 HTTP 503（AI 未配置）返回，不中断其他任务的独立请求。

#### Scenario: AI 未配置
- **WHEN** `AIService` 未配置 API key 时调用 assist 端点
- **THEN** 响应为 503，错误信息说明 AI 未配置

#### Scenario: 上游调用中途失败
- **WHEN** 已发出若干 `delta` 后上游连接中断
- **THEN** 流以 `error` 事件结束，不发送 `done`
