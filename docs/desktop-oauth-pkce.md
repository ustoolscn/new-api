# Desktop OAuth 2.0 Authorization Code + PKCE

This document describes the existing desktop-client integration. It is intended
for Hi Codex and other native desktop clients that need an API token for the
currently signed-in account.

The flow is a public OAuth client flow:

- `client_id`: `hi-codex` (fixed; do not register another client ID)
- `scope`: `api` (fixed)
- `response_type`: `code` (fixed)
- PKCE: S256 is required; `plain` is rejected
- redirect: an HTTP loopback URL on `127.0.0.1` or `::1`
- access-token lifetime: 30 days (`2,592,000` seconds)
- refresh tokens: not implemented; run the browser flow again after expiry

No custom URI protocol such as `highcodex://` is required. A conforming client
uses the loopback callback and does not require a server-side or client-side
protocol change.

## Endpoints and same-origin requirement

Let `BASE_ORIGIN` be the deployment origin that serves both the web frontend and
the API. The authorization page and its API calls use relative paths, so the
browser session, consent page, and `/api` endpoints must be same-origin (same
scheme, host, and port). A reverse proxy may route `/api` to the backend, but a
separate frontend origin and API origin must not be configured for this flow.
The default frontend provides the consent page at the `/oauth/authorize` route
and calls the same relative `/api` endpoints.
Use HTTPS for `BASE_ORIGIN` in production; the only HTTP URL in this contract
is the loopback callback.

### Browser consent page

Open this URL in the user's browser:

```text
GET BASE_ORIGIN/oauth/authorize
    ?response_type=code
    &client_id=hi-codex
    &redirect_uri=http%3A%2F%2F127.0.0.1%3A45678%2Foauth%2Fcallback
    &state=<url-encoded-state>
    &scope=api
    &code_challenge=<base64url-sha256-challenge>
    &code_challenge_method=S256
```

The URL must be URL-encoded as one query string; the line breaks above are only
for readability. The browser must have an active dashboard session. If the user
is not signed in, the frontend sends them to sign-in and preserves the original
authorization URL.

Authorization query parameters:

| Parameter | Required | Value or rule |
| --- | --- | --- |
| `response_type` | yes | Exactly `code`. |
| `client_id` | yes | Exactly `hi-codex`. |
| `redirect_uri` | yes | Valid loopback URI; see [Loopback redirect rules](#loopback-redirect-rules). |
| `state` | yes | Fresh, non-empty client value, at most 256 bytes, without control characters. |
| `scope` | yes | Exactly `api`. |
| `code_challenge` | yes | 43-character canonical unpadded base64url S256 challenge. |
| `code_challenge_method` | yes | Exactly `S256`. |

The consent page displays the signed-in account, the `api` permission, the
loopback callback, and the request expiry. The user chooses **Allow access** or
**Deny**.

### Authorization API (used by the consent page)

```text
GET BASE_ORIGIN/api/oauth/authorize
```

This endpoint has the same query parameters as the browser URL. It requires the
dashboard session cookie and does not accept an API bearer token. A successful
response uses the application's normal envelope:

```json
{
  "success": true,
  "data": {
    "request_id": "<opaque-request-id>",
    "client_id": "hi-codex",
    "client_name": "Hi Codex",
    "redirect_uri": "http://127.0.0.1:45678/oauth/callback",
    "scopes": ["api"],
    "user": {"id": 123, "username": "example"},
    "expires_at": 1800000000
  }
}
```

`expires_at` is a Unix timestamp. The authorization request is valid for 10
minutes. Desktop clients normally do not call this endpoint themselves; they
open the browser page and let the signed-in browser session call it.

### Consent decision

The consent form posts to:

```text
POST BASE_ORIGIN/api/oauth/authorize/decision
Content-Type: application/x-www-form-urlencoded
```

Canonical form fields:

| Field | Required | Value |
| --- | --- | --- |
| `request_id` | yes | The opaque ID returned by `/api/oauth/authorize`. |
| `decision` | yes | `approve` or `deny`. |

The browser session is required. The server looks up the callback URL from the
stored request; a client must not try to override it in this POST. On success,
the endpoint returns `302 Found` to the original loopback URL:

- approval: `?state=<state>&code=<one-time-code>`
- denial: `?state=<state>&error=access_denied&error_description=the+resource+owner+denied+the+request`

The query parameter order is not part of the contract. Parse the URL rather than
matching its literal text.

### Token exchange

Exchange the code at:

```text
POST BASE_ORIGIN/api/oauth/token
Content-Type: application/x-www-form-urlencoded
```

Required form fields:

| Field | Value |
| --- | --- |
| `grant_type` | `authorization_code` |
| `client_id` | `hi-codex` |
| `code` | The code received at the loopback callback. |
| `redirect_uri` | The exact same loopback URI sent in the authorization request. |
| `code_verifier` | The original PKCE verifier; never substitute the challenge. |

There is no `client_secret` for this public client.

The token endpoint deliberately returns a raw OAuth response, not the usual
`{ "success": ..., "data": ... }` application envelope.

Success (`HTTP 200`):

```json
{
  "access_token": "sk-<opaque-token>",
  "token_type": "Bearer",
  "expires_in": 2592000,
  "scope": "api"
}
```

Use the returned value as `Authorization: Bearer <access_token>` for API calls.
Store it in the operating system's secure credential store (for example,
Keychain, Windows Credential Manager, or Secret Service/libsecret), not in a
plain-text config file.

Error responses also use a raw OAuth shape and include `Cache-Control: no-store`
and `Pragma: no-cache`:

```json
{
  "error": "invalid_grant",
  "error_description": "authorization code is invalid, expired, or already used"
}
```

The main error codes are:

| Error | Typical cause |
| --- | --- |
| `invalid_request` | Missing form field, malformed request, or the per-user token limit was reached. |
| `unsupported_grant_type` | `grant_type` was not `authorization_code`. |
| `invalid_client` | `client_id` was not `hi-codex`. |
| `invalid_grant` | Invalid, expired, already-used, mismatched, or PKCE-invalid code; invalid redirect URI/verifier; or a disabled authorizing user. |
| `server_error` | The server could not issue the token. |

The token endpoint normally returns HTTP 400 for the first four client errors
and HTTP 500 for `server_error`. Do not display or log the complete verifier,
authorization code, or access token.

## Loopback redirect rules

The redirect URI must satisfy all of these rules at both authorization and token
exchange time:

- scheme is exactly `http`;
- host is exactly `127.0.0.1` (IPv4) or `::1` (IPv6);
- an explicit port from `1` through `65535` is present;
- the client binds only to that loopback address, preferably by opening the
  listener before launching the browser;
- a path is allowed, for example `/oauth/callback`;
- no query string, fragment, or userinfo is allowed;
- `localhost`, wildcard addresses such as `0.0.0.0`/`::`, external hosts, and
  HTTPS loopback URLs are rejected;
- the exact string, including host spelling, port, and path, must be reused in
  the token request.

For IPv6, bracket the address in the URI:

```text
http://[::1]:45678/oauth/callback
```

The backend appends `state`, `code`, or an error to this URI as query
parameters. Do not put a query or fragment in the registered `redirect_uri`.

## PKCE and state requirements

Generate a fresh verifier and state for every authorization attempt.

The verifier follows RFC 7636's unreserved character set (`A-Z`, `a-z`, `0-9`,
`-`, `.`, `_`, `~`) and must be 43–128 characters. A practical implementation
is 32 random bytes encoded with unpadded base64url, which produces 43
characters. Compute the challenge as:

```text
code_challenge = BASE64URL_NOPAD(SHA256(code_verifier))
```

The resulting challenge is 43 characters of canonical unpadded base64url. The
server requires `code_challenge_method=S256` and verifies the challenge with a
constant-time comparison.

`state` is required, must be non-empty, at most 256 bytes, and must not contain
control characters. After the browser redirects to the loopback listener,
compare the returned `state` with the exact value generated for this attempt
before accepting either `code` or an error. A mismatch is a failed flow; do not
redeem the code.

## Recommended desktop flow

1. Open a listener on a random free port bound only to `127.0.0.1` (or `[::1]`)
   and keep it alive for the whole authorization attempt.
2. Build the exact loopback `redirect_uri` from the bound port and path.
3. Generate a fresh `code_verifier`, S256 `code_challenge`, and random `state`.
4. URL-encode the authorization parameters and open the browser at
   `BASE_ORIGIN/oauth/authorize?...`.
5. Receive one HTTP request on the loopback listener. Parse query parameters,
   verify `state`, and handle `error=access_denied` before looking for `code`.
6. If a code is present, POST the form fields to `/api/oauth/token`, including
   the exact `redirect_uri` and the original verifier.
7. Validate `token_type`, `scope`, and `expires_in`; then store the access token
   in the OS credential store and close the listener.
8. Send API requests with `Authorization: Bearer <access_token>`. When the token
   expires, start a new browser authorization; there is currently no refresh
   token exchange.

Language-neutral pseudocode (the placeholders are secrets and must not be
written to logs):

```text
listener = listen_loopback(address = "127.0.0.1", port = 0)
redirect_uri = "http://127.0.0.1:" + listener.port + "/oauth/callback"

verifier = base64url_nopad(random_bytes(32))
challenge = base64url_nopad(sha256(verifier))
state = base64url_nopad(random_bytes(32))

open_browser(BASE_ORIGIN + "/oauth/authorize?" + url_encode({
  response_type: "code",
  client_id: "hi-codex",
  redirect_uri: redirect_uri,
  state: state,
  scope: "api",
  code_challenge: challenge,
  code_challenge_method: "S256"
}))

callback = listener.wait_for_one_request(timeout = 10 minutes)
if callback.query.state != state:
    fail("state mismatch")
if callback.query.error == "access_denied":
    cancel("user denied access")
code = callback.query.code

token = POST_FORM(BASE_ORIGIN + "/api/oauth/token", {
  grant_type: "authorization_code",
  client_id: "hi-codex",
  code: code,
  redirect_uri: redirect_uri,
  code_verifier: verifier
})
secure_store(token.access_token)
```

Use a short local success/cancel page for the callback response if desired, but
do not include the access token or verifier in that page. Stop accepting new
connections after the first callback and close the listener on timeout or
failure.

## Cancellation, expiry, and replay

- If the user selects **Deny**, the callback contains `error=access_denied` and
  the original `state`; treat this as a normal cancellation and do not call the
  token endpoint.
- A browser authorization request expires after 10 minutes. A one-time
  authorization code expires after 2 minutes.
- A code can be redeemed only once. A second redemption returns
  `invalid_grant`; it never issues a second token.
- A wrong verifier, client ID, or redirect URI fails validation. The server does
  not consume the code for those failed matches, but the client should correct
  its state and restart rather than repeatedly guessing values.
- If the browser is closed, the listener times out, or the network fails, discard
  the verifier/state and begin a fresh attempt.
- A disabled user, malformed request, or server error must be surfaced without
  retrying a terminal code. Never fall back to a custom URI scheme or print
  secrets while diagnosing failures.
