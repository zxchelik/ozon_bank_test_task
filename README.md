# URL shortener

Тестовое задание на Go: небольшой HTTP-сервис, который сохраняет длинный URL и выдает для него короткий 10-символьный код. Проект рассчитан на запуск как с PostgreSQL, так и с локальным in-memory хранилищем.

Формальный контракт API описан в [`openapi.yaml`](openapi.yaml).

Стек проекта: Go 1.25.7, `net/http`, chi, pgx, PostgreSQL 16, Goose 3.27.1 и официальный Prometheus Go client.

## Что умеет сервис

- `POST /links` создает короткую ссылку или возвращает уже существующую;
- `GET /links/{code}` возвращает оригинальный URL в JSON;
- `GET /r/{code}` перенаправляет на оригинальный URL;
- `GET /links/{code}/stats` показывает дату создания и статистику обращений;
- `/healthz`, `/readyz` и `/metrics` подходят для эксплуатации сервиса;
- каждый ответ содержит `X-Request-ID`, запросы пишутся в структурированный лог;
- приложение корректно завершает активные запросы по `SIGINT` и `SIGTERM`.

## Как создается короткая ссылка

Сначала service проверяет оригинальный URL. Допускаются только абсолютные `http` и `https` ссылки с непустым host. Пустая строка, относительный путь, другой протокол или пробелы по краям дают ошибку валидации.

После проверки генератор читает случайные байты из `crypto/rand` и собирает код длиной 10 символов. Алфавит содержит 63 символа:

```text
abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_
```

Простое вычисление `byte % 63` дало бы небольшой статистический перекос, потому что 256 не делится на 63. Поэтому значения от 252 до 255 отбрасываются, а остальные равномерно отображаются на алфавит. Пространство возможных кодов равно `63^10`, то есть `984 930 291 881 790 849` вариантов.

Полученный код передается в атомарную операцию `CreateOrGet`:

1. Если оригинальный URL уже сохранен, storage возвращает прежний код. Поэтому повторный POST идемпотентен.
2. Если URL новый и код свободен, создается запись со счетчиком обращений `0`.
3. Если сгенерированный код уже занят другой ссылкой, service генерирует следующий код.
4. После пяти последовательных collision сервис прекращает попытки и возвращает внутреннюю ошибку.

В memory-реализации целостность двух индексов (`code -> link` и `original URL -> code`) защищает `sync.RWMutex`. В PostgreSQL те же гарантии дают уникальные constraints на `original_url` и `short_code`. Одновременные запросы с одним URL не создадут несколько ссылок: проигравший конкурентную вставку запрос прочитает уже созданную запись.

При resolve PostgreSQL выполняет один `UPDATE ... RETURNING`: увеличение `access_count`, обновление `last_accessed_at` и чтение ссылки происходят атомарно. Memory storage делает те же изменения под write lock. Запрос статистики использует только чтение и счетчик не меняет.

## Структура

```text
cmd/shortener           entrypoint
internal/application    сборка зависимостей и lifecycle
internal/handler        HTTP transport
internal/service        валидация, генерация и бизнес-правила
internal/storage        контракт хранилища
internal/storage/memory in-memory реализация
internal/storage/postgres PostgreSQL реализация
internal/metrics        метрики на prometheus/client_golang
migrations              Goose-миграции
```

## Конфигурация

| Переменная | Значение по умолчанию | Назначение |
|---|---:|---|
| `HTTP_ADDR` | `:8080` | Адрес HTTP-сервера |
| `STORAGE_TYPE` | `memory` | `memory` или `postgres` |
| `POSTGRES_DSN` | — | DSN PostgreSQL, обязателен в postgres-режиме |
| `BASE_URL` | `http://localhost:8080` | Основа возвращаемого `short_url` |
| `SHUTDOWN_TIMEOUT` | `10s` | Максимальное время graceful shutdown |
| `READINESS_TIMEOUT` | `2s` | Timeout проверки PostgreSQL |

## Типовые команды

Команды разработки собраны в [`Makefile`](Makefile):

```bash
make run              # memory-режим
make build            # бинарь bin/shortener
make test
make test-race
make vet
make check
make docker-build
make postgres-up      # PostgreSQL, Goose и приложение
make postgres-down
```

Для локального управления миграциями сначала запустите БД, затем используйте Goose через Makefile:

```bash
make db-up
make migrate-up
make migrate-status
make migrate-down
```

Compose сам собирает отдельный migrator image с Goose `v3.27.1` и применяет `goose up` до запуска приложения. Локально устанавливать Goose не требуется; версию migrator можно переопределить через `GOOSE_VERSION`.
