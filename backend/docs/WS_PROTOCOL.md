# WebSocket — краткий HTTP + протокол

Полный контракт кадров и событий: [`WS_CONTRACT_V1.md`](./WS_CONTRACT_V1.md). Здесь только то, что нужно для связки HTTP ↔ WS без дублирования OpenAPI.

## 1. Получить access JWT

Через `POST /auth/login` или `POST /auth/register` (см. OpenAPI: `internal/transport/http/docs/openapi.yaml` в репозитории, на сервере — `GET /openapi.yaml`, UI: `GET /docs`).

## 2. Одноразовый ticket (HTTP, JSON API)

`POST /ws/ticket` с заголовком `Authorization: Bearer <access_jwt>`.

Успешный ответ в envelope `{ "ok": true, "data": { "ticket": "<opaque>", "expires_in": <seconds> } }`. Ticket хранится в Redis и **снимается при первом успешном consume** при connect.

## 3. Подключение WebSocket

`GET /ws/connect?ticket=<opaque>` — upgrade до WebSocket. JWT в query **не** передаётся.

До upgrade при ошибке сервер отвечает **401** с **plain text** (`missing ticket`, `invalid or expired ticket`), не в JSON-формате ошибок REST.

Успех: **101 Switching Protocols**.

## 4. После connect

Входящие/исходящие кадры — по [`WS_CONTRACT_V1.md`](./WS_CONTRACT_V1.md) (envelope, имена событий, `message.read` vs `message.read_receipt`).
