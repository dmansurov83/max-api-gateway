# План: своя реализация MAX API (user-level) на Go + интеграция с Home Assistant

**Цель:** HTTP-API для отправки сообщений в MAX от имени своего аккаунта (без регистрации бота) и подключение к Home Assistant как канал уведомлений.

---

## 1. Архитектура

```
                     ┌─────────────────┐
                     │  Home Assistant  │
                     │  (через REST)    │
                     └────────┬────────┘
                              │ POST /send
                              ▼
┌────────────┐  WebSocket  ┌──────────────────┐
│ MAX Server │ ◄─────────► │  MAX Gateway     │
│ web.max.ru │             │  (Go-сервис)     │
└────────────┘             │                  │
                           │  HTTP API :8000  │
                           └──────────────────┘
```

Один бинарник:
- **Core** — WebSocket-клиент к MAX (через `go-max-client`)
- **HTTP API** — REST-наружу (`/send`, `/health`)
- **Home Assistant** — Shell Command / RESTful Command

---

## 2. Библиотека

**`github.com/MrCatchParkington/go-max-client`** (AGPL-3.0)

Готовый WebSocket-клиент на Go. Покрытие:

| Функция | Статус |
|---------|--------|
| Auth по токену/QR | да |
| `SendMessage(chatID, text)` | да |
| Upload фото/видео/файлы | да |
| Edit/Delete/Pin | да |
| GetHistory | да |
| Управление группами | да |
| Каналы | да |
| Звонки | да |
| Автореконнект (exponential backoff) | да |
| Keepalive | да |
| Канал входящих пакетов (`Packets()`) | да |

HTTP-сервер: стандартная библиотека (`net/http`) + `chi` для роутинга (лёгкий, без лишних зависимостей).

---

## 3. Получение токена и deviceId

Открыть `https://web.max.ru` → F12 → Network → WebSocket (ws://...):
- Первое исходящее сообщение → поле `token`
- Второе → поле `deviceId`

Сохранить в `.env` или `config.yaml`.

Альтернатива — QR-авторизация (библиотека умеет): запустить сервис, отсканировать QR в приложении MAX, токен сохранится сам.

---

## 4. MVP-функционал

| № | Функция | Описание |
|---|---------|----------|
| 1 | Подключение | WebSocket к MAX с token/deviceId |
| 2 | Send message | `SendMessage(chat_id, text)` |
| 3 | HTTP endpoint | `POST /send?chat_id=X` с авторизацией по токену |
| 4 | Get chat_id | Из URL чата в web.max.ru или из событий |
| 5 | Upload file | Отправка файлов (фото, видео, документы) |
| 6 | Health check | `GET /health` для мониторинга |
| 7 | Keepalive/reconnect | Экспоненциальный backoff, keepalive-пинг |

---

## 5. Интеграция с Home Assistant

### Вариант A: RESTful Command (проще, без доп. аддонов)

```yaml
# configuration.yaml
rest_command:
  max_send:
    url: "http://192.168.1.100:8000/send"
    method: POST
    headers:
      Authorization: "ваш_токен_доступа"
    content_type: "application/json"
    payload: '{"chat_id": "-1234567890", "text": "{{ message }}"}'
```

Использование в automation:
```yaml
action:
  - service: rest_command.max_send
    data:
      message: "Дверь открыта!"
```

### Вариант B: Shell Command (проще всего)

```yaml
shell_command:
  max_notify: "curl -X POST http://192.168.1.100:8000/send -H 'Authorization: токен' -H 'Content-Type: application/json' -d '{\"chat_id\": \"-1234567890\", \"text\": \"{{ message }}\"}'"
```

### Вариант C: notify-сервис (правильный)

```yaml
# Вспомогательный скрипт или add-on, регистрирующий notify-сервис
notify:
  - name: max
    platform: rest
    method: POST
    url: "http://192.168.1.100:8000/send"
    headers:
      Authorization: "ваш_токен_доступа"
```

### Рекомендация

Начать с **Варианта A** (RESTful Command) — минимум настроек. Если потребуется больше — перейти на Вариант C (notify-сервис).

---

## 6. Структура файлов

```
├── cmd/
│   └── server/
│       └── main.go           # Точка входа
├── internal/
│   ├── config/
│   │   └── config.go         # Загрузка конфига (файл + env)
│   ├── core/
│   │   └── client.go         # Обёртка над go-max-client (подключение, реконнект, send, upload)
│   └── api/
│       ├── router.go         # Маршруты chi
│       ├── handlers.go       # HTTP-обработчики
│       └── middleware.go     # Проверка API-токена
├── config.yaml               # Конфиг
├── go.mod / go.sum
├── Dockerfile                # Двухстадийная сборка
├── docker-compose.yml        # Docker Compose с .env
├── .env.example              # Шаблон переменных
└── README.md
```

---

## 7. Пошаговый план реализации

### Шаг 1. Инициализация
```bash
mkdir max-api-self && cd max-api-self
go mod init github.com/you/max-api-self
go get github.com/MrCatchParkington/go-max-client
go get github.com/go-chi/chi/v5
go get github.com/mattn/go-sqlite3  # если нужно хранить сессии
```

### Шаг 2. Config
```go
// internal/config/config.go
type Config struct {
    Port     int    `yaml:"port"`
    Token    string `yaml:"token"`
    DeviceID string `yaml:"device_id"`
    ChatID   int64  `yaml:"default_chat_id"`
}
// Загрузка из config.yaml + env (токен можно переопределить через MAX_TOKEN)
```

### Шаг 3. Core: WebSocket-клиент
```go
// internal/core/client.go
type MaxCore struct {
    client *maxclient.Client
    cfg    *config.Config
}

func New(cfg *config.Config) *MaxCore { ... }

func (c *MaxCore) Connect(ctx context.Context) error {
    c.client = maxclient.New(maxclient.WithAutoReconnect(true))
    if err := c.client.Connect(ctx); err != nil { return err }
    return c.client.AuthToken(ctx, c.cfg.Token, c.cfg.DeviceID)
}

func (c *MaxCore) SendMessage(ctx context.Context, chatID int64, text string) error {
    _, err := c.client.SendMessage(ctx, chatID, text)
    return err
}

func (c *MaxCore) Close() { c.client.Close() }
```

### Шаг 4. HTTP API
```go
// internal/api/router.go
func NewRouter(core *core.MaxCore, apiToken string) http.Handler {
    r := chi.NewRouter()
    r.Use(middleware.Auth(apiToken))
    r.Post("/send", handlers.SendMessage(core))
    r.Get("/health", handlers.Health(core))
    return r
}
```

```go
// internal/api/handlers.go
func SendMessage(core *core.MaxCore) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        var req struct {
            ChatID int64  `json:"chat_id"`
            Text   string `json:"text"`
        }
        json.NewDecoder(r.Body).Decode(&req)
        if err := core.SendMessage(r.Context(), req.ChatID, req.Text); err != nil {
            http.Error(w, err.Error(), 500)
            return
        }
        json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
    }
}
```

### Шаг 5. Запуск
```go
// cmd/server/main.go
func main() {
    cfg := config.Load("config.yaml")
    core := core.New(cfg)
    ctx := context.Background()
    
    go func() {
        for {
            if err := core.Connect(ctx); err != nil {
                log.Printf("connect error: %v, retry in 5s", err)
                time.Sleep(5 * time.Second)
                continue
            }
            break
        }
    }()

    r := api.NewRouter(core, cfg.APIToken)
    log.Printf("listening on :%d", cfg.Port)
    http.ListenAndServe(fmt.Sprintf(":%d", cfg.Port), r)
}
```

### Шаг 6. Keepalive и реконнект
- `go-max-client` уже делает автореконнект с exponential backoff
- Добавить `time.Ticker` ~30 сек для диагностики соединения

### Шаг 7. Upload файлов
```go
func (c *MaxCore) UploadAndSend(ctx context.Context, chatID int64, path string) error {
    file, _ := os.Open(path)
    defer file.Close()
    attach, _ := c.client.UploadPhoto(ctx, filepath.Base(path), file)
    _, err := c.client.SendMessage(ctx, chatID, "", maxclient.SendMessageOpts{
        Attaches: []maxclient.Attachment{*attach},
    })
    return err
}
```

Добавить эндпоинт `POST /upload` с multipart/form-data.

---

## 8. Риски и ограничения

| Риск | Вероятность | Что делать |
|------|-------------|------------|
| Бан аккаунта за активность | Низкая (для личного use) | Не делать массовых рассылок |
| Протухание токена | Средняя | Обновлять раз в N дней; либо QR-авторизация |
| Изменение протокола MAX | Низкая → средняя | Обновлять библиотеку |
| Отвал WebSocket | Средняя | Встроенный автореконнект + checkHealth |
| Home Assistant не видит сервис | Низкая | Проверить сеть, добавить в `docker-compose` network |

---

## 9. Примеры запросов

```bash
# Отправить сообщение
curl -X POST "http://localhost:8000/send" \
  -H "Authorization: ваш_токен_доступа" \
  -H "Content-Type: application/json" \
  -d '{"chat_id": -1234567890, "text": "Уведомление из Home Assistant"}'

# Проверка здоровья
curl "http://localhost:8000/health"

# Отправить файл
curl -X POST "http://localhost:8000/upload" \
  -H "Authorization: ваш_токен_доступа" \
  -F "chat_id=-1234567890" \
  -F "file=@photo.jpg"
```

---

## 10. Оценка времени

| Шаг | Что делаем | Время |
|-----|-----------|-------|
| 1 | Инициализация Go-проекта | 15 мин |
| 2 | Config, структура проекта | 30 мин |
| 3 | Core: WebSocket-клиент + реконнект | 1 ч |
| 4 | HTTP API: handlers, middleware | 1 ч |
| 5 | Интеграция с Home Assistant | 30 мин |
| 6 | Upload файлов | 1.5 ч |
| 7 | Тестирование, отладка | 1-2 ч |

**Итого:** 5-7 часов до рабочего прототипа.

---

## 11. Что дальше (если нужно)

- Dockerfile + docker-compose (чтобы HA запускал как add-on)
- Multi-аккаунт (несколько чатов)
- Webhook на входящие сообщения (для двустороннего диалога между HA и MAX)
- Автообновление токена через QR (без ручного копирования)
- `notify:` platform для Home Assistant (вместо RESTful Command)