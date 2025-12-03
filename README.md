# Webmap 入口

一個基於 Go 語言的反向代理，支援 Discord OAuth2 身份驗證和基於角色的存取控制。
它作為入口網關，僅允許具有特定 Discord 角色的成員存取目標應用程式。

## 環境變數

- `DISCORD_CLIENT_ID`：Discord 應用客戶端 ID
- `DISCORD_CLIENT_SECRET`：Discord 應用客戶端金鑰
- `DISCORD_TOKEN`：Discord 機器人令牌
- `DISCORD_REDIRECT_URI`：基本重新導向 URI（回呼路徑會自動附加）
- `TARGET_URL`：上游服務基本 URL（例如， `https://example.com`)
- `SESSION_SECRET`：用於 cookie 會話的隨機金鑰
- 可選：`GO_ENV=development`

## 注意事項

- OAuth 回呼路徑會自動加入到 `DISCORD_REDIRECT_URI`：`/$__hook_sess___/callback`。
