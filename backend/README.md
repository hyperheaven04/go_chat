# go_chat — backend

Бэкенд диалога между двумя пользователями в реальном времени: сообщения с серверным временем,
индикатор «печатает…», отметки о прочтении, онлайн-статус и «был в сети».

**Стек:** Go 1.27 (стандартный `net/http`), PostgreSQL 17 (`pgx`), WebSocket (`coder/websocket`),
JWT (`golang-jwt`), bcrypt. Docker Compose для запуска.

## Быстрый старт

```bash
make up          # сгенерирует .env со случайными секретами и поднимет db + server на :8080
make test        # unit-тесты
make test-integration   # unit + e2e против Postgres из docker compose (все с -race)
```

## Архитектура

```
cmd/server            точка входа: конфиг, graceful shutdown
internal/
  domain              сущности, бизнес-ошибки, валидация, события (без зависимостей)
  service             сценарии: AuthService, ChatService + интерфейсы портов
  storage/postgres    репозитории и встроенные SQL-миграции
  transport/httpapi   REST: хендлеры, middleware, rate limit
  transport/ws        WebSocket: Hub (реестр соединений), Client (read/write loops)
  transport/dto       JSON-представление и маппинг ошибок
  auth                bcrypt, JWT, одноразовые WS-тикеты
  app                 сборка зависимостей (composition root)
```

Зависимости направлены внутрь: `transport → service → domain`, `storage → domain`.
Сервис не знает ни про HTTP, ни про WebSocket, ни про Postgres — только про интерфейсы
(`UserRepository`, `Notifier`, …), которые объявлены на стороне потребителя.
Поэтому сервис тестируется на in-memory фейках, а WebSocket-хаб можно заменить на Redis pub/sub,
не трогая бизнес-логику.

### Поток сообщения

```
Alice ──ws: message.send──▶ Client.readLoop ──▶ ChatService.SendMessage
                                                  ├─ проверка участия в диалоге
                                                  ├─ INSERT (created_at = now() в БД)
                                                  └─ Notifier.Publish([alice, bob], MessageCreated)
                                                           │
Hub ──▶ очередь каждого соединения Alice и Bob ──▶ Client.writeLoop ──ws: message.new──▶
```

## Ключевые решения

| Решение | Почему |
|---|---|
| **Время ставит сервер** (`DEFAULT now()` в БД), отдаётся в UTC | Часы клиентов врут и различаются; единый источник истины для порядка и отображения |
| **`client_id` + `UNIQUE (sender_id, client_id)`** | Идемпотентная отправка: повтор после обрыва связи не создаёт дубль, клиент сопоставляет «отправляется…» с подтверждённым сообщением |
| **`id bigint` для сообщений** | Монотонный порядок и курсорная пагинация (`before_id`) без проблем OFFSET |
| **Прочтение = указатель «прочитано до»** (`dialog_reads`) | Одна строка на участника вместо флага на каждом сообщении; указатель только растёт |
| **Typing не хранится** | Эфемерное событие; клиент гасит индикатор сам через ~5 c, поэтому потерянный «stop» не оставит вечное «печатает…» |
| **Диалог `user_a < user_b` + UNIQUE** | Одна пара — один диалог, даже если оба открыли его одновременно |
| **Hub на `RWMutex` + буферизованная очередь на соединение** | Рассылка никогда не блокируется медленным клиентом: переполнил очередь — отключаем, он перезагрузит историю по REST |
| **Ping каждые 25 c** | Обнаружение «мёртвых» соединений (телефон потерял сеть), иначе пользователь висел бы онлайн |
| **Мультидевайс** | У пользователя может быть несколько соединений; онлайн = хотя бы одно; offline шлётся при закрытии последнего |

## Безопасность

- **Пароли** — bcrypt (cost 12). Пароли длиннее 72 байт отклоняются: bcrypt молча обрезал бы их.
- **Нет перебора логинов**: для несуществующего пользователя выполняется фиктивное сравнение bcrypt
  (одинаковое время ответа) и возвращается та же ошибка, что для неверного пароля.
- **JWT** — HS256 с жёстко заданным алгоритмом (защита от `alg: none` и algorithm confusion),
  проверка `iss`/`aud`/`exp`; секрет ≥ 32 байт обязателен, иначе сервер не стартует.
- **WebSocket-тикеты**: браузер не умеет ставить `Authorization` на WS-handshake, а JWT в URL утёк бы
  в логи прокси. Клиент меняет JWT на тикет (`POST /ws/ticket`), который живёт 30 c и одноразовый.
  Аутентификация — до upgrade, неаутентифицированный клиент сокет не получает.
- **CSWSH**: WebSocket из браузера принимается только с разрешённого `Origin`.
- **Авторизация**: каждый запрос к диалогу проверяет участие. Чужой диалог отвечает `404`, а не `403` —
  нельзя даже узнать, существует ли он.
- **Rate limiting**: логин/регистрация — 5/мин на IP (burst 10); входящие WS-события — 10/с на соединение.
- **Лимиты размера**: тело REST ≤ 64 КБ, WS-фрейм ≤ 16 КБ, сообщение ≤ 4000 символов (и `CHECK` в БД).
- **Строгий JSON**: неизвестные поля и мусор после объекта отклоняются.
- **SQL** только параметризованный; LIKE-поиск экранирует `%` и `_`.
- **HTTP-сервер** с таймаутами (`ReadHeaderTimeout` против Slowloris), заголовки безопасности,
  CORS только по allowlist, без cookies (нет CSRF-поверхности).
- **Логи**: структурированные (slog, JSON), с `request_id`; путь логируется без query (там тикет),
  токены и пароли не логируются никогда. Наружу уходят только безопасные сообщения об ошибках.
- **Контейнер**: distroless, non-root, read-only FS, `cap_drop: ALL`, `no-new-privileges`;
  Postgres опубликован только на `127.0.0.1`.

## REST API

Все ответы — JSON, ошибки в формате `{"error": {"code", "message", "field?"}}`.
Защищённые эндпоинты требуют `Authorization: Bearer <token>`.

| Метод | Путь | Описание |
|---|---|---|
| POST | `/api/v1/auth/register` | `{username, password}` → `201 {user, token, expires_at}` |
| POST | `/api/v1/auth/login` | `{username, password}` → `{user, token, expires_at}` |
| GET | `/api/v1/users/me` | текущий пользователь |
| GET | `/api/v1/users?q=ali` | поиск по префиксу логина (до 20) |
| GET | `/api/v1/dialogs` | мои диалоги: собеседник (+online), последнее сообщение, непрочитанные |
| POST | `/api/v1/dialogs` | `{peer_id}` → диалог (`201` если создан, `200` если уже был) |
| GET | `/api/v1/dialogs/{id}` | диалог |
| GET | `/api/v1/dialogs/{id}/messages?before_id=&limit=` | история, новые первыми (limit ≤ 100) |
| POST | `/api/v1/ws/ticket` | `{ticket, expires_in}` для подключения к WS |
| GET | `/api/v1/ws?ticket=…` | WebSocket |
| GET | `/healthz`, `/readyz` | liveness / readiness (с проверкой БД) |

## WebSocket-протокол

Конверт: `{"type": "...", "ref": "optional", "payload": {...}}`. `ref` возвращается в `error`.

**Клиент → сервер**

```jsonc
{"type": "message.send", "payload": {"dialog_id": "…", "client_id": "<uuid>", "body": "Привет"}}
{"type": "typing",       "payload": {"dialog_id": "…", "active": true}}   // повторять раз в ~3 c, пока печатает
{"type": "message.read", "payload": {"dialog_id": "…", "message_id": 42}}
```

**Сервер → клиент**

```jsonc
{"type": "message.new",  "payload": {"id": 42, "dialog_id": "…", "sender_id": "…", "client_id": "…",
                                     "body": "Привет", "created_at": "2026-11-02T10:15:30.123Z"}}
{"type": "typing",       "payload": {"dialog_id": "…", "user_id": "…", "active": true}}
{"type": "message.read", "payload": {"dialog_id": "…", "user_id": "…", "message_id": 42}}
{"type": "presence",     "payload": {"user_id": "…", "online": false, "last_seen_at": "…"}}
{"type": "error",        "ref": "…", "payload": {"code": "validation_error", "message": "…", "field": "body"}}
```

`message.new` приходит и отправителю (на все его устройства) — это подтверждение доставки на сервер.
После переподключения клиент догружает пропущенное через REST.

## Конфигурация (env)

| Переменная | По умолчанию | |
|---|---|---|
| `DATABASE_URL` | — | обязательна |
| `JWT_SECRET` | — | обязательна, ≥ 32 байт |
| `HTTP_ADDR` | `:8080` | |
| `JWT_TTL` | `24h` | |
| `WS_TICKET_TTL` | `30s` | |
| `BCRYPT_COST` | `12` | 10–15 |
| `ALLOWED_ORIGINS` | пусто | origin'ы браузерных клиентов через запятую |
| `LOG_LEVEL` | `info` | |
| `SHUTDOWN_TIMEOUT` | `15s` | |

## Тесты

- `internal/domain` — валидация (границы длины, UTF-8, лимит bcrypt).
- `internal/auth` — JWT (истёкший, чужой ключ, `alg: none`, подделка), тикеты (одноразовость, TTL), bcrypt.
- `internal/service` — бизнес-правила на фейках: доступ посторонних, идемпотентность, адресаты событий,
  монотонность прочтений, гонка presence при переподключении, отсутствие user enumeration.
- `internal/app` — e2e: реальный HTTP + два WebSocket-клиента + Postgres: регистрация, диалог,
  presence, typing, сообщение с серверным временем, прочтение, история, ошибки, а также
  безопасность (чужой диалог, поддельный токен, повтор тикета, чужой Origin).

## Что бы я добавил для продакшена

- Refresh-токены с ротацией и отзывом (сейчас выход = удаление токена на клиенте, access живёт 24 ч).
- Горизонтальное масштабирование: Redis/NATS pub/sub за `Notifier`, тикеты в Redis.
- Учёт IP за доверенным reverse proxy (`X-Forwarded-For` только от него), TLS на прокси.
- Метрики Prometheus (соединения, latency, ошибки), трейсинг OpenTelemetry.
- Кэш участия в диалоге на соединении, чтобы `typing` не ходил в БД.
- Редактирование/удаление сообщений, вложения, push-уведомления для офлайн-пользователей.
