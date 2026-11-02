# go_chat

Диалог между двумя пользователями в реальном времени.

- **[backend/](backend/)** — Go-сервер: REST + WebSocket, PostgreSQL, JWT-авторизация.
  Архитектура, решения, безопасность и протокол описаны в [backend/README.md](backend/README.md).
- **[app/](app/)** — Flutter-клиент для демонстрации (Android, iOS, Web).

Функционал: регистрация и вход, поиск собеседника, история с пагинацией, сообщения с серверным
временем, «печатает…», отметки о прочтении (✓ / ✓✓), онлайн и «был(а) в сети», счётчик
непрочитанных, офлайн-очередь отправки с автоматическим переподключением.

## Запуск

Нужны Docker, Go 1.27+ (только для тестов) и Flutter 3.41+.

```bash
cd backend
make up                 # .env со случайными секретами + Postgres + сервер на :8080
make test-integration   # все тесты с -race, e2e на отдельной БД chat_test
```

Клиент:

```bash
cd app
flutter run                                          # эмулятор Android / симулятор iOS
flutter run --dart-define=API_URL=http://192.168.1.10:8080   # реальный телефон в той же сети
```

### Демо в браузере (два пользователя рядом)

Разные порты — разные origin, а значит, раздельные сессии:

```bash
# backend/.env
ALLOWED_ORIGINS=http://localhost:5100,http://localhost:5101
```

```bash
cd backend && docker compose up -d
cd ../app && flutter build web
python3 -m http.server 5100 -d build/web &
python3 -m http.server 5101 -d build/web &
```

Откройте `http://localhost:5100` и `http://localhost:5101`, зарегистрируйте двух пользователей,
найдите одного из-под другого и напишите сообщение.
