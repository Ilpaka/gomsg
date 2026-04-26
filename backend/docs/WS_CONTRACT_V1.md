# WebSocket contract v1

All server→client frames are **one JSON object per text frame** with a fixed envelope:

```json
{
  "event": "message.created",
  "data": {},
  "meta": {}
}
```

- **event** (string): outbound event name (snake_case with dots), for example `message.created`, `message.updated`, `message.deleted`, `message.read_receipt`, `typing.*`, `presence.*` (see code in `internal/transport/ws/events.go`).
- **data** (object): payload specific to the event.
- **meta** (object): optional metadata (timestamps, correlation ids); may be empty `{}`.

## Inbound commands (client → server)

Clients send JSON messages with an **event** field (command name). Important naming:

| Inbound command | Purpose |
|-----------------|--------|
| `message.read` | Mark messages read up to a given message (command). |

Do **not** confuse with outbound **`message.read_receipt`**, which notifies chat members that someone updated read state.

Other inbound commands are defined in `internal/transport/ws/events.go` (e.g. send message through WS if supported). Message mutations should go through **`MessageService`** so REST and WS share the same domain path and outbox events.

## Connection flow (browser-safe)

Long-lived JWT must **not** appear in the WebSocket URL.

1. Obtain **access** and **refresh** tokens via `POST /auth/login` or `POST /auth/register` (Bearer on subsequent HTTP calls).
2. `POST /ws/ticket` with header `Authorization: Bearer <access_jwt>`.
3. Response JSON includes a short-lived **ticket** (opaque string) and `expires_in` seconds.
4. Connect WebSocket: `GET /ws/connect?ticket=<ticket>` (rate-limited by IP).
5. The ticket is **one-time**: it is deleted from Redis on successful upgrade; reuse fails.
6. Server checks **Origin** against `ws.allowed_origins` (config / `WS_ALLOWED_ORIGINS` env, comma-separated). It is not `CheckOrigin: true`.

## Domain events and Kafka

Message lifecycle events are written to PostgreSQL **outbox** in the same transaction as the message change, then relayed to **Kafka** (or, when Kafka is disabled, fanned out locally). Another consumer in the same process reads Kafka and pushes the same envelope shape to the hub so **multiple app instances** receive consistent fan-out.

See root `README.md` for env toggles (`KAFKA_ENABLED`, `KAFKA_BROKERS`, etc.).
