# Использование Redis в GoFlow

Документ описывает **фактическое** использование Redis в backend: пакеты `internal/repository/redis`, wiring в `internal/app/container.go` и потребители в `service`, `transport/ws`, `app`.

## Используется ли Redis

**Да, обязательно для запуска приложения:** в `config.Validate()` пустой `redis.addr` приводит к ошибке (`internal/config/config.go`). Клиент создаётся в `NewContainer`, выполняется `Ping`.

## Где и для чего

| Компонент | Файл | Назначение |
|-----------|------|------------|
| Presence | `presence_repository.go` | Ключ `goflow:presence:user:{userID}`, значение `"1"`, **TTL 45s**; heartbeat продлевает TTL; `Del` при offline |
| Typing | `typing_repository.go` | Ключ `goflow:typing:{chatID}:{userID}`, TTL **8s**; явный `StopTyping` удаляет ключ |
| WS tickets | `ws_ticket_store.go` | Ключ `goflow:ws:ticket:{ticket}` → user id; **SETNX** + TTL (из конфига WS, по умолчанию 2 мин); **GetDel** при connect — одноразовое использование |
| Pub/Sub | `pubsub_repository.go` | Канал `goflow:chat:{chatID}`; `PSubscribe goflow:chat:*` в relay-горутине; payload — **готовый JSON envelope** (не хранилище сообщений) |

Потребители:

- `PresenceService` + WebSocket lifecycle (через `WSService` / hooks).
- `WSTicketService` + HTTP `POST /ws/ticket`.
- `Broadcaster.PublishToChat`: если `PubSubRepository` не nil — публикация в Redis; иначе доставка только в локальный `Hub`.
- `Broadcaster.StartRedisRelay`: подписка и проброс в `Hub` для multi-instance.

## Почему Redis не source of truth

- Пользователи, чаты, сообщения и сессии живут в **PostgreSQL** с ACID и миграциями.
- Данные в Redis **теряются** при сбросе инстанса/памяти или истечении TTL — это допустимо для presence/typing/ticket и для Pub/Sub (подписчики offline не получают прошлые сообщения из канала).

## Какие данные допустимо хранить в Redis в этом проекте

- Флаги «пользователь сейчас в онлайне» (с коротким TTL).
- Краткоживущие индикаторы набора текста.
- Одноразовые токены подключения WS.
- Транзитные broadcast payload’ы через Pub/Sub (как сигнал доставки, не как журнал).

## Какие данные нельзя считать надёжно хранимыми в Redis

- История сообщений, состав чатов, пароли, refresh-токены, outbox — всё это в **Postgres** (или в памяти процесса для rate limiter, см. ниже).

## Rate limiting

**В Redis не реализован.** Лимиты (`internal/transport/http/middleware/ratelimit.go`) — **token bucket в памяти процесса**, ключ — IP (`ClientIP`). Это значит:

- при **нескольких репликах** лимит **не глобальный** по кластеру (каждый инстанс свой bucket);
- перезапуск сбрасывает счётчики.

В корневом `README.md` Redis перечислен рядом с rate limit — это **неточность относительно кода**; источник правды — middleware.

## Pub/Sub

Используется **да**, если Redis-клиент и `PubSubRepository` подняты: `Publish` на канал чата, отдельная горутина `StartRedisRelay` с `PSubscribe`. Redis **не** гарантирует доставку offline-подписчикам и **не** дублирует Kafka: это параллельный путь для **мгновенного** fanout между процессами одного окружения.

## Жизненный цикл данных

| Данные | TTL / поведение | Конец жизни |
|--------|-----------------|-------------|
| Presence | 45s, обновляется heartbeat | Истечение TTL или `SetOffline` |
| Typing | 8s | Истечение или `StopTyping` |
| WS ticket | конфиг `ws.ticket_ttl_seconds` (сервис подставляет минимум/дефолт в store) | `GetDel` при успешном connect или истечение |
| Pub/Sub message | нет хранения | Доставлено активным подписчикам или потеряно |

## Почему для этих сценариев Redis подходит лучше PostgreSQL

- Низкая задержка и простые операции SET/EXPIRE/PUBLISH без роста OLTP-таблиц от «каждый heartbeat».
- Автоочистка через TTL без фоновых job на удаление строк presence.

## Почему Redis не заменяет Kafka здесь

- **Kafka** в проекте используется для **долговечной** очереди доменных событий между инстансами (outbox → broker → consumer).
- **Redis Pub/Sub** — fire-and-forget: нет персистентного топика уровня проекта, нет replay для нового инстанса так же, как у Kafka log.

Обе системы могут сосуществовать: Pub/Sub — быстрый путь для WS relay, Kafka — когда включена интеграция/масштабирование по событиям из outbox.

## Таблица сценариев

| Сценарий | Используется Redis? | Зачем | Почему не Postgres | Почему не Kafka | TTL / характер данных |
|----------|---------------------|-------|--------------------|-----------------|------------------------|
| Online presence | Да | Быстрый онлайн-статус | Heartbeat в таблицу = шум и нагрузка | Не нужен журнал | ~45s |
| Typing indicator | Да | Эфемерный UX-сигнал | Аналогично | Не доменное событие для лога | ~8s |
| WS connect ticket | Да | Одноразовый обмен JWT→socket | Не хранить одноразовые токены в БД | Избыточно | Задаётся конфигом |
| Межинстансовый WS fanout | Да (Pub/Sub) | Доставка envelope соседям | Не грузить БД broadcast’ами | Kafka опциональна | Нет хранения |
| Rate limit по IP | **Нет** (in-memory) | Защита HTTP/WS маршрутов | Можно было бы centralize в Redis — **не сделано** | Не уместно | N/A |
| История сообщений | Нет | — | SoT в Postgres | SoT не в Kafka value | N/A |

## Почему не PostgreSQL (раздел)

- Ephemeral объёмы обновлений (presence) плохо ложатся на нормализованную модель без агрессивной уборки и индексного давления.
- Одноразовые ticket’ы проще и дешевле инвалидировать атомарно (`GetDel`), чем вводить таблицу с TTL-cron.

## Почему не Kafka (раздел)

- Typing/presence — низкоценные сигналы; прогон через брокер добавил бы задержку и стоимость без требования к durability.
- WS ticket — строго локальная криптографическая + атомарная выдача на edge сессии.

## Риски при неправильном использовании Redis

- Хранить в Redis **единственную копию** критичных бизнес-данных → потеря при eviction/flush.
- Полагаться на Pub/Sub как на **гарантированную** доставку → сообщения теряются при сетевых разрывах или отсутствии подписчика.
- Считать rate limit per-IP общим для кластера без синхронизации → фактически лимит «на инстанс» (текущая реализация даже без Redis).

---

## Вывод

Redis в GoFlow — обязательный **ephemeral** слой: presence, typing, WS tickets и опциональный Pub/Sub для WS; он **не** заменяет PostgreSQL и **не** реализует rate limiting.

## Что важно помнить

- Очистьте устаревшую формулировку «rate limit в Redis» в маркетинговых/корневых README, если копируете архитектуру в другие документы — в коде это **не так**.

## Что можно улучшить позже

- Redis-based rate limiter для единого лимита в multi-instance.
- Явные метрики Redis (latency, errors) в Prometheus, если эксплуатация потребует.
