# 全局客户端 IP 黑名单

全局客户端 IP 黑名单用于在请求进入登录、鉴权或模型转发逻辑之前，禁止指定来源 IP 访问整个站点。它适合封禁明确的恶意来源，支持单个 IPv4、IPv6 地址和 CIDR 网段。

## 与其他 IP 设置的区别

- **全局客户端 IP 黑名单**：阻止客户端访问网页、登录、管理接口和模型接口；仅 `/api/status` 豁免。
- **API Token IP 白名单**：只限制使用某个 Token 的模型请求，不限制网页登录或其他 Token。
- **SSRF IP 黑名单**：限制服务器主动访问的出站目标，不限制访问本站的客户端。

## 配置

进入“系统设置 → 安全设置 → 客户端 IP 黑名单”：

1. 在“被阻止的客户端 IP”中每行输入一个地址或网段，例如：

   ```text
   203.0.113.7
   203.0.113.0/24
   2001:db8::/48
   ```

2. 如果服务位于反向代理后，在“可信代理 IP”中填写实际与应用建立连接的代理地址或网段。
3. 核对页面显示的“当前识别到的客户端 IP”。
4. 开启黑名单并保存。

配置保存在以下 Options 键中：

- `client_ip_setting.blacklist_enabled`
- `client_ip_setting.blacklist`
- `client_ip_setting.trusted_proxies`

当前实例保存后立即生效；多实例部署中的其他实例会按照现有 Options 同步周期生效。

## 直连部署

客户端直接连接应用时，“可信代理 IP”保持为空。此时系统只使用 TCP 直接连接地址，并忽略客户端自行提交的 `X-Forwarded-For` 和 `X-Real-IP`。

## Nginx 示例

应用只应信任实际由你控制、并直接连接应用的 Nginx 地址。不要填写 `0.0.0.0/0` 或 `::/0`。

```nginx
location / {
    proxy_pass http://127.0.0.1:3000;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $remote_addr;
}
```

如果 Nginx 与应用通过本机回环连接，可将 `127.0.0.1/32` 和需要时的 `::1/128` 加入可信代理。如果通过 Docker 网络连接，应填写 Nginx 容器实际所在的受控网段，而不是信任所有私有地址。

存在 CDN、负载均衡器和 Nginx 多层代理时，只有受控的每一层代理才能进入可信代理列表。各层必须覆盖而不是盲目追加来自公网的转发头，并及时维护 CDN 官方公布的代理网段。

## 自我封禁确认

如果新规则会包含当前识别到的管理员 IP，保存接口会要求二次确认。确认保存后，当前浏览器随后的请求也会收到 HTTP 403，这是预期行为，不存在管理员身份旁路。

## 恢复访问

如果管理员封禁了自己的 IP，可以使用以下任一方式恢复：

- 从未被黑名单覆盖的其他公网 IP 登录并修改设置。
- 通过服务器或数据库管理工具将 `client_ip_setting.blacklist_enabled` 设为 `false`。
- 修正 `client_ip_setting.blacklist` 或 `client_ip_setting.trusted_proxies` 的 JSON 数组内容，然后重启实例或等待 Options 同步。

不要通过增加秘密请求头、查询参数或 Cookie 绕过黑名单，这会形成长期安全后门。

## 性能

规则在配置加载时预编译并发布为只读内存快照。请求过程中只进行一次客户端 IP 解析和内存前缀匹配，不查询数据库、Redis 或 DNS。几十条规则下，这部分开销相对于登录、数据库操作和上游模型请求可以忽略；流式响应也只在建立请求时检查一次。
