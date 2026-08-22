# CooperAPI 视频生成 API

CooperAPI 提供统一的视频生成接口。本文介绍如何调用 `MiniMax-H3` 视频模型。

## 接口信息

```text
服务地址：https://cooper-api.com
模型名称：MiniMax-H3
鉴权方式：Bearer Token
```

请使用 CooperAPI 分配的 API Token：

```http
Authorization: Bearer YOUR_COOPERAPI_TOKEN
```

## 创建视频任务

```http
POST /v1/video/generations
Content-Type: application/json
Authorization: Bearer YOUR_COOPERAPI_TOKEN
```

### 文生视频

```bash
curl --request POST \
  --url 'https://cooper-api.com/v1/video/generations' \
  --header 'Authorization: Bearer YOUR_COOPERAPI_TOKEN' \
  --header 'Content-Type: application/json' \
  --data '{
    "model": "MiniMax-H3",
    "prompt": "一个亚洲女孩从门外走进来，看见一个穿条纹睡衣的亚洲男孩躺在沙发上。女孩生气地说：你又在躺着打游戏，有没有点正事干！男孩坐起来厌恶地说：关你屁事。两人使用中文对白，特写人物表情。",
    "resolution": "768P",
    "duration": 5,
    "ratio": "16:9"
  }'
```

### 请求参数

| 参数 | 类型 | 必填 | 说明 |
|---|---|---:|---|
| `model` | string | 是 | 固定为 `MiniMax-H3`。 |
| `prompt` | string | 是 | 视频生成提示词。 |
| `duration` | integer | 是 | 视频时长，支持 4–15 秒。 |
| `seconds` | integer/string | 否 | `duration` 的兼容写法。 |
| `resolution` | string | 否 | `768P`、`1080P` 或 `2K`，默认 `768P`。也可以使用 `size`。 |
| `size` | string | 否 | 可传 `768P`、`1080P`、`2K` 或常见尺寸，例如 `1920x1080`。 |
| `ratio` | string | 文生视频建议传 | `adaptive`、`21:9`、`16:9`、`4:3`、`1:1`、`3:4`、`9:16`。 |
| `image` | string | 否 | 单张图片 URL。 |
| `images` | array | 否 | 图片 URL 数组，或包含 `url` 和 `role` 的对象数组。 |
| `input_video` | string | 否 | 输入视频 URL。 |
| `input_videos` | array | 否 | 输入视频 URL 数组。 |
| `metadata` | object | 否 | 其他模型参数。通常不需要传递。 |

纯文生视频必须传入非 `adaptive` 的 `ratio`，例如 `16:9`。

### 图片首帧和尾帧

```bash
curl --request POST \
  --url 'https://cooper-api.com/v1/video/generations' \
  --header 'Authorization: Bearer YOUR_COOPERAPI_TOKEN' \
  --header 'Content-Type: application/json' \
  --data '{
    "model": "MiniMax-H3",
    "prompt": "人物缓慢转头看向镜头，背景树叶随风摆动。",
    "images": [
      {"url": "https://example.com/first.png", "role": "first_frame"},
      {"url": "https://example.com/last.png", "role": "last_frame"}
    ],
    "resolution": "768P",
    "duration": 5,
    "ratio": "adaptive"
  }'
```

只使用一张图片时，可以直接传 URL：

```json
{
  "model": "MiniMax-H3",
  "prompt": "让图片中的人物自然地眨眼。",
  "images": ["https://example.com/first.png"],
  "resolution": "768P",
  "duration": 5,
  "ratio": "adaptive"
}
```

### 输入视频

```json
{
  "model": "MiniMax-H3",
  "prompt": "参考输入视频的运镜节奏生成新视频。",
  "input_videos": ["https://example.com/reference.mp4"],
  "resolution": "768P",
  "duration": 5,
  "ratio": "16:9"
}
```

输入视频必须是可访问的 HTTP/HTTPS URL，不能传本地文件路径。

## 创建响应

创建成功后会返回 CooperAPI 的任务 ID：

```json
{
  "id": "task_xxxxxxxxxxxxxxxx",
  "task_id": "task_xxxxxxxxxxxxxxxx",
  "object": "video",
  "model": "MiniMax-H3",
  "status": "queued",
  "progress": 0,
  "created_at": 1785200000
}
```

后续查询和下载均使用返回的 `id` 或 `task_id`。

## 查询任务

建议每 3–10 秒查询一次：

```bash
curl --request GET \
  --url 'https://cooper-api.com/v1/video/generations/task_xxxxxxxxxxxxxxxx' \
  --header 'Authorization: Bearer YOUR_COOPERAPI_TOKEN'
```

查询响应示例：

```json
{
  "code": "success",
  "message": "",
  "data": {
    "task_id": "task_xxxxxxxxxxxxxxxx",
    "status": "SUCCESS",
    "fail_reason": "",
    "result_url": "https://cooper-api.com/v1/video/generations/task_xxxxxxxxxxxxxxxx/content",
    "progress": "100%"
  }
}
```

### 任务状态

| 状态 | 含义 | 客户端行为 |
|---|---|---|
| `NOT_START` / `SUBMITTED` / `QUEUED` | 任务已提交，等待处理 | 继续轮询 |
| `IN_PROGRESS` | 正在生成 | 继续轮询 |
| `SUCCESS` | 生成完成 | 下载视频 |
| `FAILURE` | 生成失败 | 读取 `fail_reason` 后停止轮询 |

## 下载视频

任务状态为 `SUCCESS` 后，调用内容接口下载：

```bash
curl --location \
  --url 'https://cooper-api.com/v1/video/generations/task_xxxxxxxxxxxxxxxx/content' \
  --header 'Authorization: Bearer YOUR_COOPERAPI_TOKEN' \
  --output generated-video.mp4
```

也可以使用查询响应中的 `result_url`。

## OpenAI Video 兼容接口

如果客户端使用 OpenAI Video 风格接口，也可以使用：

```http
GET /v1/videos/{task_id}
```

示例：

```bash
curl --request GET \
  --url 'https://cooper-api.com/v1/videos/task_xxxxxxxxxxxxxxxx' \
  --header 'Authorization: Bearer YOUR_COOPERAPI_TOKEN'
```

## 常见错误

- `duration` 必须是 4–15 秒的整数。
- `resolution` 只能使用 `768P`、`1080P` 或 `2K`。
- 纯文生视频需要传入合法的 `ratio`，例如 `16:9`。
- `input_video` 和 `input_videos` 必须使用 HTTP/HTTPS URL。
- `401`：Token 无效或缺少鉴权请求头。
- `404`：任务不存在，或任务不属于当前账号。
- `400`：请求参数不合法，查看响应中的错误信息。

## 安全建议

- 只在服务端保存 CooperAPI Token。
- 不要将 Token 写入前端代码、公开仓库或日志。
- 生产环境建议为每个应用单独创建 Token，并按需设置额度和权限。
