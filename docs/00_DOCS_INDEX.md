# Индекс документации GoFlow (backend)

Набор документов описывает **текущую** реализацию backend мессенджера в репозитории GoFlow (Go), а не абстрактную архитектуру.

## Созданные файлы

| Файл | Содержание |
|------|------------|
| [01_PROJECT_OVERVIEW.md](./01_PROJECT_OVERVIEW.md) | Назначение проекта, модули, потоки данных REST/WS/БД/Redis/Kafka, локальный запуск, честный статус MVP |
| [02_REDIS_USAGE.md](./02_REDIS_USAGE.md) | Реальное использование Redis (ключи, TTL, pub/sub), сравнение с Postgres/Kafka, rate limiting (**не** в Redis) |
| [03_KAFKA_USAGE.md](./03_KAFKA_USAGE.md) | Outbox, relay, топик, `DomainEvent`, consumer → WebSocket fanout, опциональность Kafka |
| [04_OBSERVABILITY_STACK.md](./04_OBSERVABILITY_STACK.md) | Prometheus, Grafana, Loki, Grafana Alloy в `docker-compose`, метрики приложения, дашборды |
| [05_DATABASE_STRUCTURES.md](./05_DATABASE_STRUCTURES.md) | Таблицы и связи по SQL-миграциям, индексы, соответствие коду |

## Рекомендуемый порядок чтения

1. **01** — контекст и карта системы (обязательно первым для нового разработчика).
2. **05** — схема данных (чтобы понимать ограничения API и outbox).
3. **02** и **03** — распределённое состояние и события (по необходимости к задаче).
4. **04** — если занимаетесь эксплуатацией, дебагом или производительностью.

## Навигация по коду (ориентиры)

| Тема | Путь в репозитории |
|------|-------------------|
| Точка входа | `backend/cmd/app/main.go` |
| DI и пулы | `backend/internal/app/container.go`, `backend/internal/app/app.go` |
| Конфиг | `backend/internal/config/config.go`, `backend/configs/local.yaml` |
| HTTP-маршруты | `backend/internal/transport/http/router.go` |
| WebSocket | `backend/internal/transport/ws/` |
| Миграции | `backend/internal/migration/*.sql`, `backend/internal/migration/runner.go` |
| Docker / observability | `backend/deployments/docker-compose.yml`, `backend/deployments/` |

## Связанные документы в репозитории (вне `docs/`)

- Корневой [README.md](../README.md) — запуск, OpenAPI, observability URLs.
- [backend/docs/WS_CONTRACT_V1.md](../backend/docs/WS_CONTRACT_V1.md) — контракт WebSocket.
- OpenAPI: `backend/internal/transport/http/docs/openapi.yaml`.

---

## Вывод

Документация в `docs/` дополняет README и привязана к фактическому коду на момент генерации.

## Что важно помнить

- Источник правды по HTTP API для клиентов — **OpenAPI**; по схеме БД — **SQL-миграции**.
- В корневом README в одном месте Redis перечислен вместе с rate limit — в коде rate limit **in-process**, не Redis (подробнее в `02_REDIS_USAGE.md`).

## Что можно улучшить позже

- Держать этот индекс в синхроне при добавлении новых разделов в `docs/`.
- Ссылаться на конкретные git-теги или версию API, если появятся релизы.
