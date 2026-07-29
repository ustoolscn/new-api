# 视频生成 API 接口文档

本文档描述 new-api 当前统一视频接口。除特别说明外，请求和响应均使用 UTF-8。

## 1. Base URL 与鉴权

将下面的 `https://your-new-api.example.com` 替换为实际部署域名：

```text
Base URL: https://your-new-api.example.com/v1
Authorization: Bearer <YOUR_API_TOKEN>
```

推荐使用的完整接口如下：

| 用途 | 方法 | 完整 URL |
|---|---|---|
| 创建视频任务 | `POST` | `https://your-new-api.example.com/v1/video/generations` |
| 查询视频任务 | `GET` | `https://your-new-api.example.com/v1/video/generations/{task_id}` |
| 下载/代理视频内容 | `GET` | `https://your-new-api.example.com/v1/video/generations/{task_id}/content` |

请求头：

```http
Authorization: Bearer YOUR_API_TOKEN
Content-Type: application/json
```

系统同时保留 OpenAI 兼容别名：

```text
POST /v1/videos
GET  /v1/videos/{task_id}
GET  /v1/videos/{task_id}/content
POST /v1/videos/{video_id}/remix
```

其中 `/v1/video/generations` 是本项目的统一、规范接口；`/v1/videos` 主要用于兼容 OpenAI Video API 客户端。

## 2. 创建视频任务

```http
POST /v1/video/generations
```

支持 `application/json` 和 `multipart/form-data`。推荐普通 URL 输入使用 JSON；只有所选渠道明确支持图片文件上传时才使用 multipart。输入视频不支持直接上传文件，必须提供可访问的 HTTP/HTTPS URL。

### 2.1 请求参数

| 参数 | 类型 | 必填 | 说明 |
|---|---|---:|---|
| `model` | string | 是 | 视频模型名称。必须是当前站点已配置、令牌分组可用并支持视频端点的模型，例如 `sora-2`、`sora-2-pro`、Veo、可灵、豆包、海螺、Vidu 或通义万相相关模型。实际列表以站点 `/v1/models` 和管理员渠道配置为准。 |
| `prompt` | string | 通常是 | 视频生成提示词。xAI 单图生视频允许省略；文生视频、参考图生视频、视频编辑和视频扩展仍需提供。其他渠道按各自校验规则处理。 |
| `seconds` | number 或 numeric string | 否 | 输出视频时长，单位秒。必须是 `1`–`3600` 的整数；具体模型通常只支持其中少数固定时长。优先使用此字段。 |
| `duration` | number 或 numeric string | 否 | `seconds` 的兼容别名。若同时提供，以 `seconds` 为准。 |
| `size` | string | 否 | 输出尺寸或清晰度，例如 `1280x720`、`720x1280`、`720p`、`1080p`。实际允许值由模型决定。 |
| `resolution` | string | 否 | `size` 的兼容别名。若同时提供，以 `size` 为准。 |
| `width` | integer | 否 | 正整数。与 `height` 同时提供且未提供 `size`/`resolution` 时，自动组合成 `{width}x{height}`。 |
| `height` | integer | 否 | 正整数。与 `width` 同时使用。 |
| `image` | string 或 object | 否 | 单张参考图。通常为 HTTP/HTTPS URL，也可按上游模型能力传 data URL。xAI 还支持 `{ "url": "..." }` 或 `{ "file_id": "..." }`。 |
| `images` | string、array | 否 | 一张或多张参考图。JSON 中可传 URL 字符串、URL 数组，或包含 `url`/`image_url.url` 与可选 `role` 的对象/对象数组。最终支持数量和角色取决于上游模型。 |
| `input_reference` | string | 否 | `image` 的兼容别名。 |
| `input_video` | string 或 object | 否 | 单个输入视频的 HTTP/HTTPS URL，用于视频续写、参考或视频到视频。xAI 还支持 `{ "url": "..." }` 或 `{ "file_id": "..." }`。不能传本地路径或 multipart 视频文件。 |
| `input_videos` | string 或 string[] | 否 | 一个或多个输入视频 URL，最多 4 个。实际提供商可能只支持 1 个或完全不支持。不要与内容不同的 `input_video` 同时传。 |
| `input_video_seconds` | number 或 numeric string | 否 | 输入视频计费时长提示，范围 `(0, 3600]`。这是兼容字段，服务端计费不能信任该值；配置了输入视频按秒计费时，服务端会从媒体元数据检测实际时长。 |
| `input_video_duration` | number 或 numeric string | 否 | `input_video_seconds` 的兼容别名。 |
| `inputVideoDuration` | number 或 numeric string | 否 | `input_video_seconds` 的 camelCase 兼容别名。 |
| `fps` | integer | 否 | 输出帧率，范围 `1`–`120`；实际可用值由模型决定。 |
| `frame_rate` | integer | 否 | `fps` 的兼容别名。 |
| `framespersecond` | integer | 否 | `fps` 的兼容别名。 |
| `framesPerSecond` | integer | 否 | `fps` 的 camelCase 兼容别名。 |
| `seed` | integer | 否 | 随机种子。相同种子不保证跨提供商得到完全相同结果。 |
| `negative_prompt` | string | 否 | 反向提示词，描述不希望出现的内容。仅支持该能力的提供商生效。 |
| `generate_audio` | boolean | 否 | 是否同时生成音频。显式 `false` 会被保留并发送；仅支持该能力的模型生效。 |
| `mode` | string | 否 | 提供商模式，例如部分可灵模型使用 `std`；xAI 输入视频设置 `extension` 时调用视频扩展，否则调用视频编辑。 |
| `metadata` | object | 否 | 提供商特有扩展参数。应传 JSON 对象，不要把通用字段重复放入其中。顶层标准字段优先于 metadata 中同名字段。 |

注意：`model` 在统一校验阶段由渠道选择逻辑使用，因此实际调用必须提供。个别旧渠道虽然能推导默认模型，也不建议省略。

### 2.2 最小 JSON 示例（文生视频）

```bash
curl --request POST \
  --url 'https://your-new-api.example.com/v1/video/generations' \
  --header 'Authorization: Bearer YOUR_API_TOKEN' \
  --header 'Content-Type: application/json' \
  --data '{
    "model": "sora-2",
    "prompt": "A cinematic tracking shot of a red fox running through a snowy pine forest",
    "seconds": 8,
    "size": "1280x720"
  }'
```

### 2.3 完整 JSON 示例（图生视频/扩展参数）

```bash
curl --request POST \
  --url 'https://your-new-api.example.com/v1/video/generations' \
  --header 'Authorization: Bearer YOUR_API_TOKEN' \
  --header 'Content-Type: application/json' \
  --data '{
    "model": "YOUR_VIDEO_MODEL",
    "prompt": "The camera slowly pushes in while the subject turns toward the sunrise",
    "negative_prompt": "blur, flicker, distorted hands, subtitles, watermark",
    "image": "https://cdn.example.com/reference.jpg",
    "seconds": 8,
    "size": "1280x720",
    "fps": 24,
    "seed": 123456,
    "generate_audio": false,
    "mode": "std",
    "metadata": {
      "quality": "high",
      "camera_control": "slow_push_in"
    }
  }'
```

`metadata` 中的字段不会保证所有渠道都透传；是否生效取决于所选提供商适配器。

### 2.4 输入视频示例

```bash
curl --request POST \
  --url 'https://your-new-api.example.com/v1/video/generations' \
  --header 'Authorization: Bearer YOUR_API_TOKEN' \
  --header 'Content-Type: application/json' \
  --data '{
    "model": "YOUR_VIDEO_TO_VIDEO_MODEL",
    "prompt": "Continue the motion naturally and transition into a wide aerial shot",
    "input_video": "https://cdn.example.com/input.mp4",
    "seconds": 8,
    "size": "1280x720"
  }'
```

输入视频 URL 必须使用 `http` 或 `https`，且目标需满足服务端 SSRF/域名/IP/端口访问策略。最多可提交 4 个 URL，但具体渠道限制可能更严格。

### 2.5 multipart 示例（渠道支持时上传图片文件）

```bash
curl --request POST \
  --url 'https://your-new-api.example.com/v1/video/generations' \
  --header 'Authorization: Bearer YOUR_API_TOKEN' \
  --form 'model=YOUR_IMAGE_TO_VIDEO_MODEL' \
  --form 'prompt=Make the clouds move and add a slow camera orbit' \
  --form 'seconds=8' \
  --form 'size=1280x720' \
  --form 'image=@reference.png;type=image/png'
```

并非所有视频适配器都会读取 multipart 图片文件；不确定时应先把图片上传到对象存储/CDN，再通过 `image` URL 调用。multipart 中 `metadata` 应作为 JSON 字符串传入，例如：

```text
metadata={"quality":"high"}
```

multipart 不接受视频文件；`input_video` 仍需传 HTTP/HTTPS URL。

### 2.6 创建成功响应

创建成功一般返回 HTTP `200` 和 OpenAI Video 风格任务对象：

```json
{
  "id": "task_xxxxxxxxxxxxxxxx",
  "task_id": "task_xxxxxxxxxxxxxxxx",
  "object": "video",
  "model": "sora-2",
  "status": "queued",
  "progress": 0,
  "created_at": 1785200000
}
```

关键字段：

| 字段 | 说明 |
|---|---|
| `id` | 对外任务 ID。后续查询和下载都使用此 ID，不要使用上游任务 ID。 |
| `task_id` | 旧接口兼容字段，通常与 `id` 相同，未来可能废弃。 |
| `object` | 固定为 `video`。 |
| `status` | `queued`、`in_progress`、`completed`、`failed` 或 `unknown`。 |
| `progress` | 整数百分比，通常为 `0`–`100`。 |
| `metadata` | 可能包含结果 URL 或提供商补充信息。 |

响应头 `X-New-Api-Other-Ratios` 可能包含本次任务计费使用的时长、分辨率等乘数，主要用于诊断，不应作为业务成功判断依据。

## 3. 查询任务状态

```http
GET /v1/video/generations/{task_id}
Authorization: Bearer YOUR_API_TOKEN
```

示例：

```bash
curl --request GET \
  --url 'https://your-new-api.example.com/v1/video/generations/task_xxxxxxxxxxxxxxxx' \
  --header 'Authorization: Bearer YOUR_API_TOKEN'
```

统一接口通常返回包装结构：

```json
{
  "code": "success",
  "message": "",
  "data": {
    "task_id": "task_xxxxxxxxxxxxxxxx",
    "platform": "sora",
    "action": "generate",
    "status": "SUCCESS",
    "fail_reason": "",
    "result_url": "https://your-new-api.example.com/v1/video/generations/task_xxxxxxxxxxxxxxxx/content",
    "submit_time": 1785200000,
    "start_time": 1785200002,
    "finish_time": 1785200045,
    "progress": "100%",
    "properties": {},
    "data": {}
  }
}
```

内部状态值：

| `data.status` | 含义 | 建议客户端行为 |
|---|---|---|
| `NOT_START` / `SUBMITTED` / `QUEUED` | 已接受，等待处理 | 继续轮询 |
| `IN_PROGRESS` | 正在生成 | 继续轮询 |
| `SUCCESS` | 生成成功 | 读取 `result_url` 或调用 content 接口 |
| `FAILURE` | 生成失败 | 显示 `fail_reason`，停止轮询 |
| `UNKNOWN` | 未知状态 | 延迟后重试，超过业务超时后停止 |

Gemini/Vertex 渠道在实时查询成功时可能返回精简的 `data` 对象，其中 `status` 为 `queued`、`processing`、`succeeded` 或 `failed`，并直接包含 `task_id`、`url`、`format`、`error` 和 `metadata`。客户端应同时兼容这两种统一查询响应，并以 `data.status` 与结果 URL 为准。

建议以 3–10 秒间隔轮询，并设置客户端总超时；不要高频无间隔查询。

如果使用 OpenAI 兼容查询接口：

```http
GET /v1/videos/{task_id}
```

则响应为扁平的 OpenAI Video 对象，状态值为 `queued`、`in_progress`、`completed`、`failed` 或 `unknown`，完成后结果地址通常位于 `metadata.url`。

## 4. 获取视频内容

任务成功后调用：

```http
GET /v1/video/generations/{task_id}/content
Authorization: Bearer YOUR_API_TOKEN
```

示例：

```bash
curl --location \
  --url 'https://your-new-api.example.com/v1/video/generations/task_xxxxxxxxxxxxxxxx/content' \
  --header 'Authorization: Bearer YOUR_API_TOKEN' \
  --output generated-video.mp4
```

该接口会验证任务归属和完成状态，再代理或解码实际视频内容。响应的 `Content-Type` 通常来自上游，例如 `video/mp4`。任务未完成时返回 `400`，任务不存在或不属于当前用户时返回 `404`。

content 接口也允许已登录控制台会话访问；API 客户端应始终使用 Bearer Token。

## 5. Remix 接口

OpenAI 兼容 remix 路径：

```http
POST /v1/videos/{video_id}/remix
```

请求示例：

```bash
curl --request POST \
  --url 'https://your-new-api.example.com/v1/videos/task_xxxxxxxxxxxxxxxx/remix' \
  --header 'Authorization: Bearer YOUR_API_TOKEN' \
  --header 'Content-Type: application/json' \
  --data '{
    "prompt": "Keep the same composition but change the weather to a thunderstorm"
  }'
```

Remix 会查找原任务、沿用其模型和渠道，并锁定到原任务渠道。只有实现了 remix 的上游才可用；原任务必须属于当前用户，且原渠道仍处于启用状态。

## 6. 错误响应

任务接口错误通常为：

```json
{
  "code": "invalid_seconds",
  "message": "seconds must be a whole number between 1 and 3600",
  "data": null
}
```

常见 HTTP 状态码和错误码：

| HTTP | 可能的错误码/场景 | 说明 |
|---:|---|---|
| `400` | `invalid_request` | JSON、参数或提供商请求不合法。 |
| `400` | `missing_model` | 未提供模型，或模型解析失败。 |
| `400` | `invalid_seconds` | 输出时长不是 1–3600 的整数。 |
| `400` | `invalid_input_video_seconds` | 输入视频时长不在 `(0, 3600]`。 |
| `400` | `invalid_fps` | 帧率不在 1–120。 |
| `400` | `invalid_input_video` | 视频 URL 非 HTTP/HTTPS、数量超过 4、字段冲突，或上传了视频文件。 |
| `400` | `unsupported_input_video` | 选中的提供商不支持输入视频。 |
| `400` | `unsupported_input_video_count` | 提供商支持的输入视频数量少于请求数量。 |
| `400` | `invalid_size` | 当前模型不支持请求尺寸。 |
| `401` | 鉴权失败 | Token 缺失、无效或已禁用。 |
| `402` / `403` | 余额、额度、权限或内容策略问题 | 根据站点配置返回。 |
| `413` | `read_request_body_failed` | 请求体超过服务端限制。 |
| `429` | 上游负载饱和或速率限制 | 稍后重试，并使用退避。 |
| `500` / `502` / `503` | 上游或服务端异常 | 可对幂等的查询请求重试；创建请求是否重试需先确认未成功创建任务。 |

content 接口采用 OpenAI 风格错误包装：

```json
{
  "error": {
    "message": "Task is not completed yet, current status: IN_PROGRESS",
    "type": "invalid_request_error"
  }
}
```

## 7. 参数优先级与兼容规则

为避免歧义，客户端最好只使用主字段。当前兼容优先级如下：

```text
seconds > duration
size > resolution > width + height
fps > frame_rate > framespersecond > framesPerSecond
input_video_seconds > input_video_duration > inputVideoDuration
image > input_reference
input_video > video
```

顶层 `negative_prompt`、`generate_audio`、`fps`、`seed` 和 `input_video_seconds` 会被规范化进内部 metadata；若 metadata 中也存在同名值，顶层标准字段生效。

## 8. Sora 兼容行为

当前统一校验对 `sora-2` 系列有以下默认和限制：

| 模型 | 默认时长 | 默认尺寸 | 允许尺寸 |
|---|---:|---|---|
| `sora-2` | 4 秒 | `720x1280` | `720x1280`、`1280x720` |
| `sora-2-pro` | 4 秒 | `720x1280` | `720x1280`、`1280x720`、`1792x1024`、`1024x1792` |

其他模型的时长、尺寸、参考图数量、输入视频、音频和扩展参数能力由对应上游决定。统一接口接受某字段，不代表每一个视频模型都支持该字段。

## 9. 豆包 Seedance 2.0 参数说明

当前豆包视频适配器内置以下 Seedance 2.0 模型名：

```text
doubao-seedance-2-0-260128
doubao-seedance-2-0-fast-260128
```

适配器实际向豆包上游提交：

```text
POST {豆包渠道 Base URL}/api/v3/contents/generations/tasks
```

### 9.1 推荐直接使用的顶层参数

| new-api 参数 | 转换后的豆包参数 | Seedance 2.0 用途 |
|---|---|---|
| `model` | `model` | 模型 ID。标准版使用 `doubao-seedance-2-0-260128`，快速版使用 `doubao-seedance-2-0-fast-260128`。 |
| `prompt` | `content[].type=text` | 文本提示词；适配器会把它放到 content 数组最后。 |
| `image` / `images` | `content[].type=image_url` | 图片输入。每个值转换为 `{ "type":"image_url", "image_url":{"url":"..."} }`。 |
| `input_video` / `input_videos` | `content[].type=video_url` | 视频输入 URL。每个值转换为 `{ "type":"video_url", "video_url":{"url":"..."} }`。 |
| `seconds` | `duration` | 输出时长，转换为整数。统一接口要求 1–3600 的整数，但 Seedance 实际可用时长仍以上游模型限制为准。 |
| `size` | `resolution` | 输出分辨率。`1280x720` 会按短边归一化为 `720p`，`1920x1080` 归一化为 `1080p`，`3840x2160` 归一化为 `2160p`；如需明确传 `4k`，直接使用 `size: "4k"`。 |
| `seed` | `seed` | 随机种子。 |
| `generate_audio` | `generate_audio` | 是否生成音频；显式 `false` 会正常透传。 |
| `metadata` | 豆包扩展字段 | 用于传递下表中的原生字段。 |

### 9.2 通过 metadata 传递的豆包原生参数

```json
{
  "metadata": {
    "ratio": "16:9",
    "camera_fixed": false,
    "watermark": false,
    "return_last_frame": true,
    "service_tier": "default",
    "execution_expires_after": 3600,
    "draft": false,
    "priority": 0,
    "safety_identifier": "user-123",
    "frames": 0,
    "callback_url": "https://example.com/video-callback",
    "tools": [
      { "type": "web_search" }
    ]
  }
}
```

当前适配器能识别的 metadata 字段：

| metadata 字段 | 类型 | 说明 |
|---|---|---|
| `ratio` | string | 输出画面比例，例如 `16:9`、`9:16`。具体枚举由豆包模型决定。 |
| `resolution` | string | 原生分辨率。若顶层同时传了 `size`，最终以顶层 `size` 转换出的 resolution 为准。 |
| `duration` | integer | 原生时长。若顶层传了 `seconds`，最终以 `seconds` 为准。 |
| `seed` | integer | 原生种子。若顶层传了 `seed`，最终以顶层值为准。 |
| `generate_audio` | boolean | 是否生成音频。若顶层传了 `generate_audio`，最终以顶层值为准。 |
| `camera_fixed` | boolean | 是否固定摄像机。 |
| `watermark` | boolean | 是否添加水印。 |
| `return_last_frame` | boolean | 是否在结果中返回末帧。 |
| `service_tier` | string | 豆包服务等级。可能影响调度或价格，应按实际账号能力填写。 |
| `execution_expires_after` | integer | 任务执行过期时间。单位和范围由豆包上游定义。 |
| `draft` | boolean | 是否使用草稿模式。 |
| `priority` | integer | 任务优先级。 |
| `safety_identifier` | string | 终端用户安全标识，建议使用稳定但不直接暴露隐私的用户 ID。 |
| `frames` | integer | 输出帧数。不要在不了解模型约束时与 `duration` 混用。 |
| `callback_url` | string | 豆包上游回调地址。注意：new-api 本身仍会保存并轮询任务，使用回调不替代本地查询接口。 |
| `tools` | array | 豆包工具配置；每项当前仅转换 `type`。 |
| `content` | array | 原生多模态内容，可包含 `text`、`image_url`、`video_url`、`audio_url` 和 `role`。顶层 prompt/image/input_video 会覆盖 content 中对应类型。 |

`metadata.model` 会被主动删除，不能通过 metadata 绕过模型选择或计费。

### 9.3 当前不会传给 Seedance 2.0 的通用字段

虽然统一视频请求可以接收下列字段，但当前豆包适配器的上游请求结构不包含它们，因此调用 Seedance 2.0 时不会生效：

| 字段 | 当前行为 |
|---|---|
| `negative_prompt` | 会进入内部 metadata，但豆包请求结构没有此字段，最终不会传给上游。 |
| `fps` / `frame_rate` / `framesPerSecond` | 会进入内部 metadata，但豆包请求结构使用的是 `frames`，不是 fps，最终不会作为帧率传递。 |
| `mode` | 当前豆包适配器不读取。 |
| `width` / `height` | 只用于组合成 `size`，之后按短边转换为 `resolution`；不会作为独立宽高传递。 |
| 任意未知 metadata 字段 | JSON 转换到固定的豆包请求结构时会被忽略。 |

### 9.4 Seedance 2.0 推荐请求

标准版文生视频：

```bash
curl --request POST \
  --url 'https://your-new-api.example.com/v1/video/generations' \
  --header 'Authorization: Bearer YOUR_API_TOKEN' \
  --header 'Content-Type: application/json' \
  --data '{
    "model": "doubao-seedance-2-0-260128",
    "prompt": "电影感航拍镜头，一辆越野车沿着雪山公路行驶，云层快速流动，光影自然，镜头平稳推进",
    "seconds": 5,
    "size": "1080p",
    "seed": 123456,
    "generate_audio": true,
    "metadata": {
      "ratio": "16:9",
      "camera_fixed": false,
      "watermark": false,
      "return_last_frame": true
    }
  }'
```

快速版图生视频：

```bash
curl --request POST \
  --url 'https://your-new-api.example.com/v1/video/generations' \
  --header 'Authorization: Bearer YOUR_API_TOKEN' \
  --header 'Content-Type: application/json' \
  --data '{
    "model": "doubao-seedance-2-0-fast-260128",
    "prompt": "人物缓慢转头看向镜头，头发被微风吹动，背景光线自然变化，保持人物身份和服装一致",
    "image": "https://cdn.example.com/seedance-reference.jpg",
    "seconds": 5,
    "size": "720p",
    "generate_audio": false,
    "metadata": {
      "ratio": "9:16",
      "camera_fixed": true,
      "watermark": false
    }
  }'
```

多模态参考（图片、视频、音频）可以直接使用原生 content：

```json
{
  "model": "doubao-seedance-2-0-260128",
  "prompt": "参考素材的动作节奏和环境音，生成连贯的新镜头",
  "seconds": 5,
  "size": "720p",
  "metadata": {
    "ratio": "16:9",
    "content": [
      {
        "type": "image_url",
        "image_url": { "url": "https://cdn.example.com/reference.jpg" },
        "role": "reference_image"
      },
      {
        "type": "video_url",
        "video_url": { "url": "https://cdn.example.com/reference.mp4" },
        "role": "reference_video"
      },
      {
        "type": "audio_url",
        "audio_url": { "url": "https://cdn.example.com/reference.mp3" },
        "role": "reference_audio"
      }
    ]
  }
}
```

如果使用原生 `metadata.content`，不要再传顶层 `image` 或 `input_video`，否则同类型 content 会被顶层字段替换。顶层 `prompt` 始终会替换 content 中原有的 text 项。

### 9.5 当前计费识别

当前代码对 Seedance 2.0 标准版按“输出分辨率档 + 是否包含视频输入”计算额外倍率：

| 模型 | 分辨率档 | 无视频输入基准价 | 有视频输入价 |
|---|---|---:|---:|
| 标准版 | 480p/720p | 46 | 28 |
| 标准版 | 1080p | 51 | 31 |
| 标准版 | 4K | 26 | 16 |
| Fast | 基准档 | 37 | 22 |

这些数值在代码中用于计算相对于基准价的 `video_input` 倍率，单位注释为“元/百万 token”。Fast 模型没有配置 1080p/4K 的独立倍率；请求这类组合时适配器按基准倍率预扣，是否接受由豆包上游决定。

计费识别分辨率时只把精确的 `1080p` 和 `4k` 视为特殊档位。因此若要使用 4K 计费档，应传 `size: "4k"`，不要传 `3840x2160`，因为后者会被归一化为 `2160p`，当前价格表不会把 `2160p` 识别为 `4k`。

## 10. xAI Grok Imagine Video 参数说明

xAI 渠道内置 `grok-imagine-video` 和 `grok-imagine-video-1.5`。适配器会向 `POST /v1/videos/generations` 提交普通生成，向 `POST /v1/videos/edits` 提交视频编辑，向 `POST /v1/videos/extensions` 提交视频扩展，并通过 `GET /v1/videos/{request_id}` 轮询。

主要转换规则：

| new-api 参数 | xAI 参数或行为 |
|---|---|
| `seconds` / `duration` | 转换为整数 `duration`。普通生成范围 `1`–`15`，默认 `8`；扩展范围 `2`–`10`，默认 `6`。 |
| `size: "1280x720"` | 转换为 `resolution: "720p"` 和 `aspect_ratio: "16:9"`。也可直接传 `resolution`、`aspect_ratio`。 |
| 单个 `image` 或单个 `images` | 转换为 xAI `image`，执行图生视频。图片可使用 URL、Base64 data URL 或 `file_id`。 |
| `reference_images` 或 2–7 个 `images` | 转换为 xAI `reference_images`。`grok-imagine-video-1.5` 当前不支持参考图模式。 |
| `video` / `input_video` | 默认转换为 xAI `video` 并调用视频编辑接口。仅支持一个输入视频。 |
| `mode: "extension"` | 有输入视频时调用视频扩展接口。`extend`、`video_extension` 也视为扩展模式。 |
| `output`、`storage_options`、`user` | 作为 xAI 官方字段直接转发；也可放在 `metadata` 中。 |

xAI 视频请求必须使用 `application/json`。适配器不会转发 `fps`、`seed`、`negative_prompt`、`generate_audio` 等 xAI 官方视频接口未定义的字段。完整对话、生图和生视频说明见 `docs/xai-channel.md`。

## 11. 高级自定义渠道的视频任务配置

渠道类型 `58`（Advanced Custom）可以通过 `other_settings.advanced_custom.video_task` 配置异步视频任务，不再返回 `invalid api platform: 58`。配置包含三部分：

- `submit`：创建任务的上游方法、路径、鉴权、请求头和 JSON 请求体模板。
- `query`：轮询任务的上游方法、路径、鉴权、请求头和可选 JSON 请求体模板。
- `response`：从上游 JSON 中提取任务 ID、状态、进度、结果 URL 和错误信息。

完整配置示例：

```json
{
  "advanced_custom": {
    "advanced_routes": [],
    "video_task": {
      "submit": {
        "method": "POST",
        "path": "/v1/videos/generations",
        "auth": {
          "type": "header",
          "name": "Authorization",
          "value": "Bearer {api_key}"
        },
        "headers": {
          "Accept": "application/json"
        },
        "body": {
          "model": "{model}",
          "prompt": "{prompt}",
          "duration": "{duration}",
          "image": "{image}",
          "images": "{images}",
          "input_video": "{input_video}",
          "resolution": "{metadata.resolution}",
          "aspect_ratio": "{metadata.aspect_ratio}"
        }
      },
      "query": {
        "method": "GET",
        "path": "/v1/videos/{task_id}",
        "auth": {
          "type": "header",
          "name": "Authorization",
          "value": "Bearer {api_key}"
        }
      },
      "response": {
        "task_id_path": "request_id",
        "status_path": "status",
        "progress_path": "progress",
        "result_url_path": "video.url",
        "error_path": "error.message",
        "status_map": {
          "pending": "QUEUED",
          "processing": "IN_PROGRESS",
          "done": "SUCCESS",
          "failed": "FAILURE"
        }
      }
    }
  }
}
```

如果上游请求字段与统一接口基本一致，可以把提交模板简化为：

```json
"body": "{request}"
```

`{request}` 会展开成规范化后的完整请求对象，并将 `metadata` 中的扩展字段同时合并到顶层；标准字段会覆盖 metadata 中的同名字段。

### 11.1 模板变量

提交接口可使用：

```text
{request} {model} {prompt} {mode}
{image} {images} {input_reference}
{input_video} {input_videos}
{seconds} {duration} {size} {width} {height}
{fps} {seed} {negative_prompt} {generate_audio}
{metadata} {metadata.xxx} {api_key} {origin_task_id}
```

查询接口可使用：

```text
{task_id} {model} {action} {api_key}
```

当 JSON 字段值完全等于一个模板变量时，数组、对象、数字、布尔值会保留原始 JSON 类型；可选值不存在或为空时，该字段会从请求体中省略。模板变量嵌入普通字符串时会转换为字符串，例如 `"Bearer {api_key}"` 或 `"/tasks/{task_id}"`。

### 11.2 鉴权配置

`auth.type` 支持：

| 值 | 行为 |
|---|---|
| 不配置 `auth` | 默认发送 `Authorization: Bearer {api_key}`。 |
| `header` | 使用 `name` 指定请求头名，`value` 支持模板变量。 |
| `query` | 将鉴权值加入 URL 查询参数。 |
| `none` | 不自动添加鉴权。 |

`headers` 可补充任意固定或模板化请求头。`Content-Type` 在存在 JSON 请求体时默认为 `application/json`。

### 11.3 响应路径与状态映射

响应路径使用 GJSON 点路径，例如：

```text
request_id
data.task.id
data.output.0.url
video.url
error.message
```

`task_id_path` 和 `status_path` 必填。`status_map` 的目标值只能是：

```text
SUBMITTED
QUEUED
IN_PROGRESS
SUCCESS
FAILURE
```

没有显式配置的常见状态仍会自动识别，例如 `pending`、`queued`、`processing`、`running`、`completed`、`done`、`failed` 和 `cancelled`。无法识别的状态会保留任务等待下次轮询，并在后台记录解析错误。

提交上游可返回任意 `2xx` 状态码，包括常见的 `200`、`201` 和 `202`。高级自定义视频任务当前只接受 URL 形式的图片和视频输入，不转发 multipart 文件；文件应先上传到对象存储或 CDN。

## 12. 推荐调用流程

1. 调用 `POST /v1/video/generations`，保存响应中的 `id`。
2. 每 3–10 秒调用 `GET /v1/video/generations/{id}`。
3. 状态为 `SUCCESS` 后，优先使用 `result_url`；也可调用 `GET /v1/video/generations/{id}/content`。
4. 状态为 `FAILURE` 时记录 `fail_reason`，不要继续轮询。
5. 对 `429` 和临时 `5xx` 使用指数退避；不要盲目重试创建请求，以免产生重复任务和重复计费。
