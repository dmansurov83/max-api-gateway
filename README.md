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

### Вариант C: notify-сервис (правильный)

```yaml
notify:
  - name: max
    platform: rest
    method: POST
    url: "http://192.168.1.100:8000/send"
    headers:
      Authorization: "ваш_api_token"
```

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