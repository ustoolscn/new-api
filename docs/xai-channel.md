# xAI 渠道适配说明

本文档对应 xAI 官方 `https://docs.x.ai/llms.txt`（核对日期：2026-07-29），说明 new-api 的 xAI 渠道如何转发对话、生图和生视频请求。

## 渠道配置

```text
渠道类型：xAI
Base URL：https://api.x.ai
Key：xAI API Key
```

请求使用：

```http
Authorization: Bearer <XAI_API_KEY>
Content-Type: application/json
```

## 对话模型

当前内置模型包括：

```text
grok-4.5
grok-4.5-latest
grok-4.3
grok-4.3-latest
grok-4.20-0309-reasoning
grok-4.20-0309-non-reasoning
grok-4.20-multi-agent-0309
grok-build-0.1
```

原有 Grok 4.1、Grok 4 Fast、Grok 3 和兼容别名继续保留。

支持的 new-api 接口：

```text
POST /v1/chat/completions
POST /v1/responses
POST /v1/responses/compact
```

xAI 官方推荐优先使用 Responses API。`/v1/responses` 会按官方格式转发 `input`、`instructions`、`tools`、`reasoning`、`previous_response_id`、`store`、`include`、`prompt_cache_key` 等字段。`/v1/responses/compact` 直接转发到 xAI 的同名上下文压缩接口。

Chat Completions 示例：

```json
{
  "model": "grok-4.5",
  "messages": [
    {
      "role": "user",
      "content": "解释一下量子纠缠"
    }
  ],
  "stream": true
}
```

Responses 示例：

```json
{
  "model": "grok-4.5",
  "input": "查询今天的重要科技新闻",
  "tools": [
    { "type": "web_search" },
    { "type": "x_search" }
  ],
  "store": false
}
```

## 生图模型

当前内置模型：

```text
grok-imagine-image-quality
grok-imagine-image
grok-imagine-image-pro
grok-2-image-1212
```

其中当前官方主模型为 `grok-imagine-image-quality` 和 `grok-imagine-image`；旧模型名继续保留，方便已有渠道配置使用。

支持接口：

```text
POST /v1/images/generations
POST /v1/images/edits
```

支持参数：

| 参数 | 说明 |
|---|---|
| `model` | 模型名称。 |
| `prompt` | 生成或编辑提示词。 |
| `n` | 输出图片数量。 |
| `aspect_ratio` | `1:1`、`3:4`、`4:3`、`9:16`、`16:9`、`2:3`、`3:2`、`9:19.5`、`19.5:9`、`9:20`、`20:9`、`1:2`、`2:1`、`auto`。 |
| `resolution` | `1k` 或 `2k`。 |
| `response_format` | `url` 或 `b64_json`。 |
| `image` | 单图编辑输入，支持 URL、Base64 data URL 或 `file_id` 对象。 |
| `images` | 多图编辑输入，与 `image` 互斥。 |
| `storage_options` | xAI Files 存储选项。 |
| `user` | 最终用户标识。 |

生图示例：

```json
{
  "model": "grok-imagine-image-quality",
  "prompt": "A futuristic city skyline at sunset",
  "n": 2,
  "aspect_ratio": "16:9",
  "resolution": "2k",
  "response_format": "url"
}
```

单图编辑示例：

```json
{
  "model": "grok-imagine-image-quality",
  "prompt": "Render this as a pencil sketch",
  "image": {
    "url": "https://example.com/source.png"
  },
  "resolution": "2k"
}
```

多图编辑示例：

```json
{
  "model": "grok-imagine-image-quality",
  "prompt": "Put <IMAGE_0> into the clothing shown in <IMAGE_1>",
  "images": [
    { "url": "https://example.com/person.png" },
    { "file_id": "file_clothing" }
  ],
  "aspect_ratio": "3:4"
}
```

兼容转换：`size: "1024x1024"` 会转换为 `aspect_ratio: "1:1"` 和 `resolution: "1k"`；建议直接传 xAI 原生的 `aspect_ratio` 与 `resolution`。multipart 图片编辑会转成 Base64 data URL 后，以 JSON 请求发送给 xAI。

## 生视频模型

当前内置模型：

```text
grok-imagine-video
grok-imagine-video-1.5
```

new-api 统一接口：

```text
POST /v1/video/generations
GET  /v1/video/generations/{task_id}
GET  /v1/video/generations/{task_id}/content
```

也可以使用 OpenAI 兼容别名：

```text
POST /v1/videos
GET  /v1/videos/{task_id}
GET  /v1/videos/{task_id}/content
```

适配器根据输入自动选择 xAI 上游接口：

| 请求类型 | 判断方式 | xAI 上游接口 |
|---|---|---|
| 文生视频 | 不传图片和视频 | `POST /v1/videos/generations` |
| 图生视频 | 传单个 `image` 或单个 `images` | `POST /v1/videos/generations` |
| 参考图生视频 | 传 `reference_images`，或 `images` 中包含 2–7 张图 | `POST /v1/videos/generations` |
| 视频编辑 | 传 `video`/`input_video`，不指定扩展模式 | `POST /v1/videos/edits` |
| 视频扩展 | 传 `video`/`input_video`，并设置 `mode: "extension"` | `POST /v1/videos/extensions` |
| 查询任务 | 使用内部保存的上游 `request_id` | `GET /v1/videos/{request_id}` |

生成参数：

| 参数 | 说明 |
|---|---|
| `model` | `grok-imagine-video` 或 `grok-imagine-video-1.5`。 |
| `prompt` | 文生视频和参考图生视频必填；单图生视频可以省略。 |
| `duration` / `seconds` | 普通生成 `1`–`15` 秒，默认 `8` 秒。视频扩展 `2`–`10` 秒，默认 `6` 秒。 |
| `aspect_ratio` | `1:1`、`16:9`、`9:16`、`4:3`、`3:4`、`3:2`、`2:3`。 |
| `resolution` | `480p`、`720p` 或 `1080p`。 |
| `image` | 单张图片。支持 URL、Base64 data URL 或 `{ "file_id": "..." }`。 |
| `reference_images` | 参考图数组，最多 7 张；当前仅 `grok-imagine-video` 支持。 |
| `images` | 兼容字段：1 张按 `image` 处理，2–7 张按 `reference_images` 处理。 |
| `video` / `input_video` | 视频编辑或扩展输入。支持 HTTP/HTTPS URL 或 `file_id` 对象。 |
| `mode` | 传 `extension`/`extend`/`video_extension` 时使用视频扩展，否则输入视频按视频编辑处理。 |
| `output` | xAI 签名上传 URL 配置。 |
| `storage_options` | xAI Files 存储配置。 |
| `user` | 最终用户标识。 |

文生视频示例：

```json
{
  "model": "grok-imagine-video",
  "prompt": "A rocket launching from Mars",
  "duration": 8,
  "aspect_ratio": "16:9",
  "resolution": "720p"
}
```

图生视频示例：

```json
{
  "model": "grok-imagine-video-1.5",
  "image": {
    "url": "https://example.com/first-frame.png"
  },
  "duration": 8,
  "resolution": "1080p"
}
```

参考图生视频示例：

```json
{
  "model": "grok-imagine-video",
  "prompt": "Use the person and clothing from the references",
  "reference_images": [
    { "url": "https://example.com/person.png" },
    { "file_id": "file_clothing" }
  ],
  "duration": 8,
  "aspect_ratio": "16:9",
  "resolution": "720p"
}
```

视频编辑示例：

```json
{
  "model": "grok-imagine-video",
  "prompt": "Give the woman a silver necklace",
  "input_video": "https://example.com/input.mp4"
}
```

视频扩展示例：

```json
{
  "model": "grok-imagine-video",
  "prompt": "The camera pans to reveal a sunset",
  "input_video": "https://example.com/input.mp4",
  "mode": "extension",
  "duration": 6
}
```

xAI 视频任务提交后返回异步 `request_id`。new-api 不向客户端暴露该上游 ID，而是返回本地 `task_...` ID，并使用提交时实际选中的 xAI Key 轮询任务。任务完成后，签名视频 URL 会保存为结果地址，content 接口负责代理输出。

当前 xAI 视频适配器不会向上游发送 `fps`、`seed`、`negative_prompt`、`generate_audio` 等非官方字段。
