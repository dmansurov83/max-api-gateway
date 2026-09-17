> **Примечание:** Весь код в этом репозитории полностью сгенерирован ИИ.

# MAX Gateway

HTTP-API для отправки сообщений в MAX от имени своего аккаунта.
Интеграция с Home Assistant для уведомлений.

## Быстрый старт

### 1. Получить токен и deviceId

Открыть `https://web.max.ru` → F12 → Network → WebSocket (ws://...):
- Первое исходящее сообщение → поле `token`
- Второе → поле `deviceId`

### 2. Настроить конфиг

```bash
cp config.yaml config.local.yaml
# Заполнить max_token, max_device_id, api_token
```

Или через переменные окружения:

```bash
export MAX_TOKEN=ваш_токен
export MAX_DEVICE_ID=ваш_device_id
export API_TOKEN=секрет_для_http
```

### 3. Запустить

```bash
go run ./cmd/server
```

### 4. Отправить сообщение

```bash
curl -X POST http://localhost:8000/send \
  -H "Authorization: ваш_api_token" \
  -H "Content-Type: application/json" \
  -d '{"chat_id": -1234567890, "text": "Привет из MAX Gateway!"}'
```

chat_id можно получить из URL чата в web.max.ru (число после `/chat/`).

## Интеграция с Home Assistant

### Вариант A: RESTful Command (рекомендуется)

```yaml
# configuration.yaml
rest_command:
  max_send:
    url: "http://192.168.1.100:8000/send"
    method: POST
    headers:
      Authorization: "ваш_api_token"
    content_type: "application/json"
    payload: '{"chat_id": -1234567890, "text": "{{ message }}"}'
  max_send_photo:
    url: "http://192.168.1.100:8000/upload"
    method: POST
    headers:
      Authorization: "ваш_api_token"
    content_type: "multipart/form-data"
    payload: '{"chat_id": -1234567890}'
```

Automation:

```yaml
action:
  - service: rest_command.max_send
    data:
      message: "Дверь открыта!"
```

### Вариант B: Shell Command (без доп. настроек)

```yaml
shell_command:
  max_notify: "curl -X POST http://192.168.1.100:8000/send -H 'Authorization: ваш_api_token' -H 'Content-Type: application/json' -d '{\"chat_id\": -1234567890, \"text\": \"{{ message }}\"}'"
```

### Вариант C: notify-сервис `notify.max` (правильный)

Готовый кастомный компонент лежит в `custom_components/max_notify`. Скопируйте папку в `config/custom_components/` вашего Home Assistant и добавьте в `configuration.yaml`:

```yaml
notify:
  - platform: max_notify
    name: max
    url: "http://192.168.1.100:8000"
    api_key: "ваш_api_token"
    chat_id: -1234567890
```

`chat_id` можно опустить — тогда он задаётся в самом запросе. После перезагрузки Home Assistant появится сервис `notify.max`:

```yaml
action:
  - service: notify.max
    data:
      title: "Дом"
      message: "Дверь открыта!"
```

**Отправка картинки** (например, снимок с камеры) — передайте путь к файлу в `image`:

```yaml
action:
  - service: notify.max
    data:
      message: "Снимок с камеры"
      image: "/config/www/snapshot.jpg"
```

`image` принимает и URL (`http://...` / `https://...`) — компонент сам скачает файл и отправит:

```yaml
action:
  - service: notify.max
    data:
      message: "Снимок с камеры"
      image: "http://192.168.1.50:8123/local/snapshot.jpg"
```

Файл уходит на `POST /upload` (multipart) как `file`, поддерживаются jpg/png/gif/webp.

## API

### `POST /send`

```json
{"chat_id": -1234567890, "text": "сообщение"}
```

### `POST /upload`

Multipart: `chat_id` (text) + `file` (file)

### `GET /health`

```json
{"status": "ok", "connected": true}
```

## Docker

```dockerfile
FROM golang:1.27 AS build
WORKDIR /app
COPY . .
RUN CGO_ENABLED=0 go build -o server ./cmd/server

FROM alpine:latest
COPY --from=build /app/server /server
COPY config.yaml /config.yaml
EXPOSE 8000
CMD ["/server"]
```

```bash
docker build -t max-gateway .
docker run -e MAX_TOKEN=... -e MAX_DEVICE_ID=... -e API_TOKEN=... -p 8000:8000 max-gateway
```

### Сборка прямо из GitHub (docker compose)

Compose умеет собирать образ прямо из git-репозитория — клонировать локально не нужно. Токены задаются через `.env` рядом с `docker-compose.yml`:

```bash
# .env
MAX_TOKEN=ваш_токен
MAX_DEVICE_ID=ваш_device_id
API_TOKEN=секрет_для_http
```

```bash
docker compose up -d --build
```

Compose-файл указывает на `https://github.com/dmansurov83/max-api-gateway.git#master` — при каждом `--build` Docker тянет свежий `master` из GitHub и собирает образ. Учтите: изменения попадут в сборку только после `git push`.

## Структура

```
├── cmd/server/main.go       # Точка входа
├── internal/
│   ├── config/config.go     # Конфиг (yaml + env)
│   ├── core/client.go       # WebSocket-клиент MAX
│   └── api/
│       ├── router.go        # Маршруты
│       ├── handlers.go      # HTTP-обработчики
│       └── middleware.go    # Авторизация
├── config.yaml              # Шаблон конфига
└── go.mod
```