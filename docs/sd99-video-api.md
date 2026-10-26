# New API 调用 SD99 TC 2.0 视频模型接口文档

更新日期：2026-10-27（北京时间）  
模型：video-sd-2.0(TC)  
适用版本：已包含 SD99 独立通道（类型 59）的 New API。

本文面向通过我们平台调用模型的开发者，覆盖请求参数、十种生成案例、任务查询、视频下载及可运行的调用示例。调用地址填写你部署的 New API 服务地址，认证使用在本平台创建的 API Key。SD99 供应商密钥由管理员保存在渠道配置中。

## 1. 调用前准备

| 项目 | 调用方填写 |
| --- | --- |
| 服务根地址 | https://your-new-api.example.com，替换为我们平台实际地址 |
| API Key | 本平台生成的 sk-... 令牌 |
| 模型名 | video-sd-2.0(TC)，管理员配置别名时使用对外别名 |
| Content-Type | application/json |
| 任务执行方式 | 异步创建、轮询状态、下载视频 |
| 首次联调案例 | 5 秒、480p、16:9、无参考素材 |

管理员需先配置 SD99 渠道、模型分组和按次或按秒价格，确保调用令牌有目标模型及分组的使用权限，并已设置正确的对外服务地址。后台配置见 [SD99 视频通道](sd99-video.md)。

所有示例中的域名、令牌、任务号、响应时间和媒体 URL 均为占位值。参考媒体须替换为真实可访问的 HTTPS 直链。创建视频可能扣费，请选择一个案例手动提交，不要批量运行全部案例。

## 2. 接口清单

| 操作 | 方法 | 路径 | 返回内容 |
| --- | --- | --- | --- |
| 查看令牌可用模型 | GET | /v1/models | 模型列表 |
| 创建视频，统一入口 | POST | /v1/video/generations | 视频任务 JSON |
| 创建视频，兼容入口 | POST | /v1/videos | 与统一创建入口相同 |
| 查询任务，视频格式 | GET | /v1/videos/{id} | 顶层 status、progress、metadata |
| 查询任务，通用格式 | GET | /v1/video/generations/{id} | code + data，状态位于 data.status |
| 下载视频 | GET | /v1/videos/{id}/content | 视频二进制 |
| 下载视频，统一入口 | GET | /v1/video/generations/{id}/content | 同上 |

建议创建使用统一入口，查询与下载使用 /v1/videos 路径，方便客户端统一处理视频格式。两个创建地址二选一，同一个业务请求只 POST 一次。

每次请求均携带：

```http
Authorization: Bearer YOUR_NEW_API_KEY
```

创建额外携带 Content-Type: application/json。查询和下载使用创建响应中的公开 id（task_...）；不要改用 SD99 上游任务号、数据库数字 ID 或自己生成的 ID。查询和下载校验任务所属用户，使用创建任务的同一账号下有效 API Key。

## 3. 创建请求字段

### 3.1 推荐请求结构

```json
{
  "model": "video-sd-2.0(TC)",
  "prompt": "黄昏海边，海浪轻拍沙滩，镜头缓慢推进。",
  "duration": 5,
  "metadata": {
    "ratio": "16:9",
    "resolution": "480p",
    "generate_audio": false
  }
}
```

| 字段 | 类型 | 必填 | 范围及默认行为 |
| --- | --- | --- | --- |
| model | string | 是 | 精确模型名 video-sd-2.0(TC)，保留括号和大小写；或管理员提供的别名 |
| prompt | string | 是 | 非空提示词；描述主体、场景、动作、镜头、风格与声音需求 |
| duration | integer | 否 | 普通生成 5–15 秒，省略默认 5；编辑模式为 -1 |
| metadata | object | 否 | 视频扩展参数容器 |
| metadata.ratio | string | 否 | 普通生成支持 16:9、9:16、1:1、4:3、3:4、21:9；默认 16:9；编辑必须 adaptive |
| metadata.resolution | string | 否 | 480p、720p、1080p，小写 p；默认 480p |
| metadata.generate_audio | boolean | 否 | true 开启声音生成，false 关闭；省略使用上游默认策略 |
| metadata.content | array | 否 | 图片、视频和音频参考，纯文生视频可省略 |

建议显式填写时长、比例和分辨率。推荐结构中的 duration 使用 JSON 数字，声音开关使用布尔值。不要发送 "default" 等界面状态字符串。

未列出的 metadata 扩展参数不会透传。本通道不支持自定义 FPS、seed、negative_prompt、multipart 文件上传及 /remix；这些字段不能作为本模型可用功能来依赖。

### 3.2 参考素材格式

| 素材 | type | role | URL 字段 | 数量限制 |
| --- | --- | --- | --- | --- |
| 图片 | image_url | reference_image | image_url.url | 最多 9 张 |
| 视频 | video_url | reference_video | video_url.url | 普通生成最多 3 段；编辑恰好 1 段 |
| 音频 | audio_url | reference_audio | audio_url.url | 最多 3 段，必须同时提供图片或视频 |

每项只放一种媒体，使用与类型匹配的 role。普通参考图均使用 reference_image，不支持把它当成已确认的首尾帧控制接口。相同 URL 会去重并保持素材顺序；用提示词说明各素材用途，具体生成效果由上游模型决定。

素材 URL 必须是 HTTPS 媒体直链，不能是 file://、本地路径、blob:、需要浏览器登录的页面或未公开的临时地址。保留签名 URL 的完整查询参数，并让有效期覆盖排队和生成。普通参考素材需要供应商可访问；编辑源片还需要本平台服务器能访问并读取媒体时长。输入文件体积、编码格式和素材总时长的供应商限制需以账号实际支持为准。

声音生成开关控制输出声音，不等同于移除参考视频原声。只有音频、没有图片或视频的请求会被拒绝。

### 3.3 统一字段写法

现有客户端也可以使用以下统一字段，示例见案例 10：

| 统一字段 | 转换行为 |
| --- | --- |
| seconds | 输出时长，可用整数数字或数字字符串；同时提供 duration 时优先使用 seconds |
| size | 可用 480p、720p、1080p，或按短边归档的像素尺寸，例如 1280x720 对应 720p |
| ratio | 未提供 metadata.ratio 时使用 |
| images / image | 转为 reference_image 素材 |
| input_videos / input_video | 转为 reference_video 素材 |
| input_audios / input_audio | 转为 reference_audio 素材 |
| generate_audio | 顶层提供时覆盖 metadata.generate_audio |

metadata.resolution 优先于 size；比例由 ratio 控制，不能只靠像素尺寸推断横竖屏。统一字段中的素材会追加到 metadata.content，再做去重和总数量校验。建议每次请求选一种清晰写法，避免重复提供或相互矛盾的参数。

## 4. 完整请求案例

所有案例均 POST 到同一个创建地址。下列 JSON 也以独立文件提供在 examples/sd99/ 目录中，使用前修改素材 URL。

### 4.1 文生视频 480p

最小普通生成案例，声音策略由上游默认决定。

请求文件：[01-text-480p.json](examples/sd99/01-text-480p.json)。

```json
{
  "model": "video-sd-2.0(TC)",
  "prompt": "黄昏的海边，海浪轻拍沙滩，镜头缓慢向前推进，写实电影质感。",
  "duration": 5,
  "metadata": {
    "ratio": "16:9",
    "resolution": "480p"
  }
}
```

### 4.2 文生视频 720p 并生成声音

显式开启输出声音，声音描述写入提示词。

请求文件：[02-text-720p-audio.json](examples/sd99/02-text-720p-audio.json)。

```json
{
  "model": "video-sd-2.0(TC)",
  "prompt": "清晨森林中鸟儿掠过溪流，镜头平稳跟随，伴随自然鸟鸣和流水声。",
  "duration": 10,
  "metadata": {
    "ratio": "16:9",
    "resolution": "720p",
    "generate_audio": true
  }
}
```

### 4.3 竖屏视频 1080p 并关闭声音

比例为 9:16；generate_audio=false 会保留并发送上游。

请求文件：[03-text-1080p-vertical.json](examples/sd99/03-text-1080p-vertical.json)。

```json
{
  "model": "video-sd-2.0(TC)",
  "prompt": "城市夜景中的咖啡杯产品特写，镜头缓慢环绕，霓虹光线映照杯身。",
  "duration": 5,
  "metadata": {
    "ratio": "9:16",
    "resolution": "1080p",
    "generate_audio": false
  }
}
```

### 4.4 单张图片参考

图片用于参考主体和场景；需要替换为真实图片直链。

请求文件：[04-single-image.json](examples/sd99/04-single-image.json)。

```json
{
  "model": "video-sd-2.0(TC)",
  "prompt": "以参考图片中的小屋为主体，保持建筑外观一致，海风吹动窗帘，镜头缓慢推进。",
  "duration": 5,
  "metadata": {
    "ratio": "16:9",
    "resolution": "720p",
    "content": [
      {
        "type": "image_url",
        "role": "reference_image",
        "image_url": {
          "url": "https://media.example.com/house.jpg"
        }
      }
    ]
  }
}
```

### 4.5 多张图片参考

素材顺序对应提示词中的第一、第二、第三张图；不承诺特定身份还原或画面结果。

请求文件：[05-multiple-images.json](examples/sd99/05-multiple-images.json)。

```json
{
  "model": "video-sd-2.0(TC)",
  "prompt": "参考第一张图片中的人物外观、第二张图片中的服装和第三张图片中的场景，人物沿街缓慢行走，保持身份一致。",
  "duration": 8,
  "metadata": {
    "ratio": "16:9",
    "resolution": "720p",
    "content": [
      {
        "type": "image_url",
        "role": "reference_image",
        "image_url": {
          "url": "https://media.example.com/person.jpg"
        }
      },
      {
        "type": "image_url",
        "role": "reference_image",
        "image_url": {
          "url": "https://media.example.com/clothing.jpg"
        }
      },
      {
        "type": "image_url",
        "role": "reference_image",
        "image_url": {
          "url": "https://media.example.com/street.jpg"
        }
      }
    ]
  }
}
```

### 4.6 视频运镜参考

这是参考生成：输出时长为 6 秒，不要求跟随参考视频长度。

请求文件：[06-video-reference.json](examples/sd99/06-video-reference.json)。

```json
{
  "model": "video-sd-2.0(TC)",
  "prompt": "借鉴参考视频的缓慢推进运镜，生成全新的黄昏海岸场景。",
  "duration": 6,
  "metadata": {
    "ratio": "16:9",
    "resolution": "720p",
    "content": [
      {
        "type": "video_url",
        "role": "reference_video",
        "video_url": {
          "url": "https://media.example.com/camera-motion.mp4"
        }
      }
    ]
  }
}
```

### 4.7 图片与音频参考

音频搭配图片使用，满足音频参考必须有视觉素材的要求。

请求文件：[07-image-audio.json](examples/sd99/07-image-audio.json)。

```json
{
  "model": "video-sd-2.0(TC)",
  "prompt": "以参考图片中的海边小屋为场景，让灯光渐亮，参考音频的宁静氛围，生成相应环境声。",
  "duration": 5,
  "metadata": {
    "ratio": "16:9",
    "resolution": "720p",
    "generate_audio": true,
    "content": [
      {
        "type": "image_url",
        "role": "reference_image",
        "image_url": {
          "url": "https://media.example.com/house.jpg"
        }
      },
      {
        "type": "audio_url",
        "role": "reference_audio",
        "audio_url": {
          "url": "https://media.example.com/ambience.mp3"
        }
      }
    ]
  }
}
```

### 4.8 图片视频音频混合参考

同一次请求包含三种媒体，各素材分别一项。

请求文件：[08-mixed-references.json](examples/sd99/08-mixed-references.json)。

```json
{
  "model": "video-sd-2.0(TC)",
  "prompt": "参考图片中的小屋外观、视频中的推进运镜与音频氛围，生成黄昏海边的电影感短片。",
  "duration": 10,
  "metadata": {
    "ratio": "16:9",
    "resolution": "720p",
    "generate_audio": true,
    "content": [
      {
        "type": "image_url",
        "role": "reference_image",
        "image_url": {
          "url": "https://media.example.com/house.jpg"
        }
      },
      {
        "type": "video_url",
        "role": "reference_video",
        "video_url": {
          "url": "https://media.example.com/camera-motion.mp4"
        }
      },
      {
        "type": "audio_url",
        "role": "reference_audio",
        "audio_url": {
          "url": "https://media.example.com/ambience.mp3"
        }
      }
    ]
  }
}
```

### 4.9 编辑原视频

源视频必须为 4–15 秒，恰好一段；ratio 必须为 adaptive。本平台先探测真实时长，探测失败时拒绝创建。按秒计费使用真实时长向上取整后的秒数，例如 8.2 秒按 9 秒计算输出时长，发送上游仍为 -1。可添加范围内的图片或音频辅助参考。编辑能力需由实际供应商账号验收。

请求文件：[09-edit-video.json](examples/sd99/09-edit-video.json)。

```json
{
  "model": "video-sd-2.0(TC)",
  "prompt": "编辑参考视频：将背景替换为黄昏海边，保持人物、动作、镜头节奏与原片时长。",
  "duration": -1,
  "metadata": {
    "ratio": "adaptive",
    "resolution": "720p",
    "content": [
      {
        "type": "video_url",
        "role": "reference_video",
        "video_url": {
          "url": "https://media.example.com/source-4-to-15-seconds.mp4"
        }
      }
    ]
  }
}
```

### 4.10 统一字段兼容写法

与前面案例走相同 SD99 通道。size=1280x720 仅确定 720p 档位，画面比例由 metadata.ratio 确定。

请求文件：[10-unified-fields.json](examples/sd99/10-unified-fields.json)。

```json
{
  "model": "video-sd-2.0(TC)",
  "prompt": "参考图片中的小屋外观和视频运镜，结合参考音频，生成宁静海边场景。",
  "seconds": "5",
  "size": "1280x720",
  "images": [
    "https://media.example.com/house.jpg"
  ],
  "input_videos": [
    "https://media.example.com/camera-motion.mp4"
  ],
  "input_audios": [
    "https://media.example.com/ambience.mp3"
  ],
  "generate_audio": false,
  "metadata": {
    "ratio": "16:9"
  }
}
```

## 5. 创建响应

创建成功时 HTTP 200，返回视频任务对象。以下为结构示例：

```json
{
  "id": "task_example_public_id",
  "task_id": "task_example_public_id",
  "object": "video",
  "model": "video-sd-2.0(TC)",
  "status": "queued",
  "progress": 0,
  "created_at": 1793059200
}
```

保存 id 作为后续查询和下载凭据。task_id 是兼容字段，值与 id 相同。created_at 为 Unix 秒时间戳。创建返回 queued 只表示提交成功，不代表视频已经完成，也不承诺队列等待时间。

客户端应先保存自己的业务请求记录，拿到任务号后立即持久化。当前接口未提供已确认的幂等请求头；POST 超时、断线、5xx 或缺少任务号时不要直接重发。即使服务器配置了重试，本通道一旦开始向上游发送 POST 就不会自动重放。

## 6. 轮询任务

### 6.1 视频格式

```http
GET /v1/videos/task_example_public_id
Authorization: Bearer YOUR_NEW_API_KEY
```

执行中示例：

```json
{
  "id": "task_example_public_id",
  "task_id": "task_example_public_id",
  "object": "video",
  "model": "video-sd-2.0(TC)",
  "status": "in_progress",
  "progress": 30,
  "created_at": 1793059200
}
```

成功示例：

```json
{
  "id": "task_example_public_id",
  "task_id": "task_example_public_id",
  "object": "video",
  "model": "video-sd-2.0(TC)",
  "status": "completed",
  "progress": 100,
  "created_at": 1793059200,
  "completed_at": 1793059290,
  "metadata": {
    "url": "https://your-new-api.example.com/v1/video/generations/task_example_public_id/content"
  }
}
```

metadata.url 是本站成品代理入口，仍需要认证；不要把它当成永久公开文件链接。可直接通过自己的 BASE_URL 与公开 id 构造下载路径。进度是状态提示，不是供应商精确进度，不应以 progress=100 代替 status 判断。

| status | 意义 | 下一步 |
| --- | --- | --- |
| queued | 待开始或排队 | 继续查询原任务 |
| in_progress | 执行中 | 继续查询原任务 |
| completed | 生成完成 | 下载原任务 |
| failed | 已确认失败 | 展示 error，停止普通轮询 |
| unknown | 本站任务状态未知 | 保留任务号，记录异常并核对，不重建任务 |

失败响应在视频对象中携带：

```json
{
  "id": "task_example_public_id",
  "task_id": "task_example_public_id",
  "object": "video",
  "model": "video-sd-2.0(TC)",
  "status": "failed",
  "progress": 100,
  "created_at": 1793059200,
  "completed_at": 1793059290,
  "error": {
    "code": "video_generation_failed",
    "message": "供应商返回的任务失败原因"
  }
}
```

客户端可每次查询响应后等待 15 秒再查，这只是建议间隔。查询读取本站后台轮询已保存的状态，不保证每次客户端 GET 都会实时访问供应商。HTTP 查询失败时可以对原任务退避重查。

### 6.2 通用任务格式

GET /v1/video/generations/{id} 返回通用封装，读取 data.status。下面只展示需要使用的字段，实际响应还包含其他任务信息：

```json
{
  "code": "success",
  "message": "",
  "data": {
    "task_id": "task_example_public_id",
    "platform": "59",
    "status": "SUCCESS",
    "progress": "100%",
    "result_url": "https://your-new-api.example.com/v1/video/generations/task_example_public_id/content",
    "fail_reason": ""
  }
}
```

通用状态为 NOT_START、SUBMITTED、QUEUED、IN_PROGRESS、SUCCESS、FAILURE 等，progress 是带百分号的字符串。code=success 表示查询请求成功，只有 data.status=SUCCESS 才表示生成成功。

## 7. 下载成品

```http
GET /v1/videos/task_example_public_id/content
Authorization: Bearer YOUR_NEW_API_KEY
```

成功返回视频字节，常见 Content-Type 为 video/mp4；按二进制保存，不要调用 response.json()。视频未完成时下载返回 400；没有任务或任务属于其他用户时返回 404；上游下载异常、空文件或返回 HTML/JSON 错误页时通常返回 502。

下载失败只重试原任务的下载，不重新 POST。建议及时保存至自己的对象存储，本站视频代理不会自动归档成品，供应商临时链接的有效期没有保证。

## 8. PowerShell 手动调用

请从包含本文件和 examples 目录的文档根目录运行。先在本机环境变量中安全设置 NEW_API_BASE_URL、NEW_API_KEY；BASE_URL 使用服务根地址，不额外附加 /v1。

```powershell
$baseUrl = $env:NEW_API_BASE_URL.TrimEnd('/')
$headers = @{ Authorization = "Bearer $env:NEW_API_KEY" }

# 只执行一次创建；此操作可能收费。
$body = Get-Content -LiteralPath './examples/sd99/01-text-480p.json' -Raw -Encoding utf8
$task = Invoke-RestMethod -Method Post -Uri "$baseUrl/v1/video/generations" -Headers $headers -ContentType 'application/json; charset=utf-8' -Body ([System.Text.Encoding]::UTF8.GetBytes($body))
$task | ConvertTo-Json -Depth 20 | Set-Content -LiteralPath './created-task.json' -Encoding utf8
$taskId = $task.id
if (-not $taskId) { throw '未取得任务号，请先核对提交结果，勿自动重发。' }

# 查询原任务；在后续操作中保留上面的 taskId。
$status = Invoke-RestMethod -Method Get -Uri "$baseUrl/v1/videos/$taskId" -Headers $headers
$status | ConvertTo-Json -Depth 20

# 只在 completed 后下载。
if ($status.status -eq 'completed') {
  Invoke-WebRequest -Method Get -Uri "$baseUrl/v1/videos/$taskId/content" -Headers $headers -OutFile './generated-video.mp4'
}
```

替换创建时读取的 JSON 文件即可使用其他案例。发生创建异常时不要重新执行整段脚本；先核对后台和供应商记录。文件 created-task.json 中保存的 id 可用于单独查询和下载。

## 9. cURL 手动调用

以下为 Bash 示例，环境变量需由本机安全配置。

```bash
# 只创建一次，替换 JSON 文件名可选择其他案例。
curl --fail-with-body --silent --show-error --max-time 180 \
  -X POST "${NEW_API_BASE_URL%/}/v1/video/generations" \
  -H "Authorization: Bearer ${NEW_API_KEY}" \
  -H 'Content-Type: application/json' \
  --data-binary '@examples/sd99/01-text-480p.json' \
  -o created-task.json

# 从 created-task.json 中取得 id 后替换下面的值。
TASK_ID='task_replace_with_actual_id'
curl --fail-with-body --silent --show-error --max-time 30 \
  "${NEW_API_BASE_URL%/}/v1/videos/${TASK_ID}" \
  -H "Authorization: Bearer ${NEW_API_KEY}"

# 仅在 completed 后下载；不要为下载失败重新创建任务。
curl --fail-with-body --silent --show-error --max-time 600 \
  "${NEW_API_BASE_URL%/}/v1/videos/${TASK_ID}/content" \
  -H "Authorization: Bearer ${NEW_API_KEY}" \
  -o generated-video.mp4
```

## 10. Python 服务端示例

附带 [sd99_client.py](examples/sd99/sd99_client.py)，仅依赖 Python 标准库，可分别执行创建、查询、等待和下载。创建没有自动重试；创建响应会先保存至指定任务文件。不要把服务端 API Key 写入前端源码。

```powershell
# 环境变量 NEW_API_BASE_URL 和 NEW_API_KEY 已安全配置。
# 从文档根目录运行，只选择一个 create 命令。
python ./examples/sd99/sd99_client.py create ./examples/sd99/01-text-480p.json --task-file ./created-task.json

# 根据任务文件查询；任务文件没有有效 id 时停止，不重新创建。
python ./examples/sd99/sd99_client.py status --task-file ./created-task.json

# 等待最多 1800 秒；到时只停止本地等待，可再次执行同一命令。
python ./examples/sd99/sd99_client.py wait --task-file ./created-task.json --timeout 1800 --interval 15

# 生成完成后单独下载。
python ./examples/sd99/sd99_client.py download --task-file ./created-task.json --output ./generated-video.mp4
```

创建请求前，脚本会用排他方式预留任务文件，若文件已存在则拒绝再次创建；请为新的业务生成指定新的文件名。文件记录 submitting 或 submission_unconfirmed 且没有 id 时，先核对后台记录，不要删除后直接重发。脚本不是生产级幂等系统；服务器进程崩溃、响应丢失等情况仍须人工核对。

## 11. 错误和重试

创建或查询失败可能返回任务错误结构，例如：

```json
{
  "code": "invalid_seconds",
  "message": "SD99 duration must be an integer from 5 to 15, or -1 for editing",
  "data": null
}
```

鉴权、限流、分发和视频下载错误也可能使用 error 对象或其他现有中间件封装。先检查 HTTP 状态，再兼容读取 message、error.message；不要假设所有失败都具有完全相同的 JSON 外层。

| 情况 | 处理方式 |
| --- | --- |
| 400 参数错误 | 根据 message 修正时长、比例、分辨率或参考格式 |
| 模型无权限、分组不可用、余额不足 | 检查令牌权限、分组、渠道和定价；按实际 HTTP 状态及业务错误判断 |
| 创建超时、断线、5xx、submission_unconfirmed | 提交结果可能未知；保存业务记录，核对后再决定是否发起新任务 |
| 查询临时超时或 HTTP 异常 | 退避后 GET 同一个 id |
| 上游 fail_to_fetch_task 或未知状态 | 本站保留旧任务继续后台查询；客户端不创建替代任务 |
| 本地等待超时 | 停止当前等待，保留原 id，可稍后恢复查询 |
| status=failed | 展示任务错误；再次生成属于新的收费操作 |
| completed 后下载失败 | 重试下载原任务，必要时联系管理员检查结果地址 |

SD99 任务不会仅因等待超过本站通用任务时限而自动失败退款；已确认失败时沿用本站任务退款规则。本站退费与供应商账单退款是两套记录，不能混同。

## 12. 费用和接入边界

按次价格按一次任务计费；按秒价格依据管理员配置的输出分辨率、输出时长及可选输入素材费用计算。普通生成使用经过校验的 5–15 秒，编辑使用探测源片真实时长向上取整。客户端声明的输入视频时长不作为可信计费依据。

参考生成是依据素材生成新视频，编辑模式则使用 -1 与 adaptive 跟随源片；两者用途不同。支持的接口与参数由当前适配代码限定，真实账号的模型授权、各分辨率、声音生成及多素材、编辑效果需在供应商账号下分别验收。

所有案例为调用模板，并非真实生成记录。本站已实现后台持久化和轮询，但未知提交不具备已确认的上游幂等能力；创建响应丢失、进程退出或保存失败时应人工核对。长期成品存储由调用方完成。

