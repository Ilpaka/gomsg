# HTTP API (OpenAPI)

Исходный файл спецификации лежит рядом с кодом, который его отдаёт: `internal/transport/http/docs/openapi.yaml` (встраивается через `go:embed` при сборке).

В запущенном приложении тот же YAML доступен по **`GET /openapi.yaml`**, интерактивно — **`GET /docs`** (Swagger UI).
