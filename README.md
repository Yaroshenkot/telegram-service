```markdown
# Telegram Service

Сервис для управления несколькими независимыми соединениями с Telegram через gRPC API. Реализован на Go с использованием библиотеки `github.com/gotd/td`.

## 📋 Содержание
- [Соответствие требованиям ТЗ](#соответствие-требованиям-тз)
- [Архитектура](#архитектура)
- [Структура проекта](#структура-проекта)
- [Требования](#требования)
- [Установка и запуск](#установка-и-запуск)
- [Конфигурация](#конфигурация)
- [API методы](#api-методы)
- [Примеры использования](#примеры-использования)
- [Описание архитектурных решений](#описание-архитектурных-решений)
- [Возможные проблемы](#возможные-проблемы)

## ✅ Соответствие требованиям ТЗ

| Требование | Реализация |
|------------|------------|
| **Динамическое создание соединений** | `CreateSession` - создаёт сессию и возвращает QR-код |
| **Динамическое удаление соединений** | `DeleteSession` - останавливает сессию и вызывает `auth.logOut` |
| **Отправка текстовых сообщений** | `SendMessage` - отправляет сообщения через авторизованную сессию |
| **Получение текстовых сообщений** | `SubscribeMessages` - стриминг входящих сообщений |
| **Изоляция соединений** | Каждая сессия в отдельной горутине со своим клиентом |
| **gRPC API** | Полностью реализован на основе предоставленного .proto |
| **Библиотека gotd/td** | Используется для всех Telegram операций |
| **In-memory хранение** | Сессии хранятся в памяти (map + mutex) |
| **Конфигурация** | Через переменные окружения |

## 🏗 Архитектура

Проект построен на чистой архитектуре с разделением ответственности:

```

┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│   gRPC Server   │────▶│ Session Manager │────▶│    Session      │
│  (api/grpc)     │     │  (manager.go)   │     │  (session.go)   │
└─────────────────┘     └─────────────────┘     └─────────────────┘
│                         │
▼                         ▼
┌─────────────────┐     ┌─────────────────┐
│   Config        │     │  Telegram API   │
│  (config.go)    │     │  (gotd/td)      │
└─────────────────┘     └─────────────────┘

```

### Компоненты

1. **gRPC сервер** (`asd/api/grpc/server.go`) - обрабатывает внешние запросы
2. **Менеджер сессий** (`asd/session/manager.go`) - управляет жизненным циклом сессий
3. **Сессия** (`asd/session/session.go`) - обёртка над Telegram клиентом
4. **Конфигурация** (`asd/config/config.go`) - загрузка настроек

## 📁  Структура проекта

```

telegram_project/
├── cmd/
│   └── server/
│       └── main.go                    # Точка входа
├── asd/                                # Internal пакеты
│   ├── api/
│   │   └── grpc/
│   │       ├── server.go               # gRPC сервер
│   │       └── proto/                   # Protocol Buffers
│   │           ├── telegram.proto
│   │           ├── telegram.pb.go
│   │           └── telegram_grpc.pb.go
│   ├── config/
│   │   └── config.go                    # Конфигурация
│   └── session/
│       ├── manager.go                    # Менеджер сессий
│       └── session.go                    # Реализация сессии
├── go.mod
├── go.sum
└── README.md

```

## 📦 Требования

- **Go:** версия 1.21 или выше
- **protoc:** Protocol Buffers compiler
- **Telegram API credentials:** api_id и api_hash (получить на https://my.telegram.org)

## 🔧  Установка и запуск

### 1. Клонирование репозитория

```bash
git clone https://github.com/yourusername/telegram_project.git
cd telegram_project
```

2. Установка зависимостей

```bash
go mod tidy
```

3. Установка protoc (если не установлен)

Ubuntu/Debian:

```bash
sudo apt install protobuf-compiler
```

macOS:

```bash
brew install protobuf
```

4. Установка плагинов protoc

```bash
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
export PATH="$PATH:$(go env GOPATH)/bin"
```

5. Генерация protobuf (если нужно)

```bash
protoc --go_out=. --go-grpc_out=. asd/api/grpc/proto/telegram.proto
```

6. Запуск сервера

```bash
export APP_ID=ваш_api_id
22:06
export APP_HASH=ваш_api_hash
export PORT=50051
go run cmd/server/main.go
```

⚙️  Конфигурация

Переменная Описание Обязательная По умолчанию
APP_ID ID приложения из my.telegram.org ✅ Да -
APP_HASH Хеш приложения из my.telegram.org ✅ Да -
PORT Порт gRPC сервера ❌ Нет 50051

📡  API методы

Сгенерированный .proto файл

```protobuf
edition = "2023";

package pact.telegram;

service TelegramService {
    rpc CreateSession(CreateSessionRequest) returns (CreateSessionResponse);
    rpc DeleteSession(DeleteSessionRequest) returns (DeleteSessionResponse);
    rpc SendMessage(SendMessageRequest) returns (SendMessageResponse);
    rpc SubscribeMessages(SubscribeMessagesRequest) returns (stream MessageUpdate);
}

message CreateSessionRequest {}

message CreateSessionResponse {
    string session_id = 1;
    string qr_code = 2;
}

message DeleteSessionRequest {
    string session_id = 1;
}

message DeleteSessionResponse {}

message SendMessageRequest {
    string session_id = 1;
    string peer = 2;
    string text = 3;
}

message SendMessageResponse {
    int64 message_id = 1;
}

message SubscribeMessagesRequest {
    string session_id = 1;
}

message MessageUpdate {
    int64 message_id = 1;
    string from = 2;
    string text = 3;
    int64 timestamp = 4;
}
```

💻 Примеры использования

Установка grpcurl

```bash
go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest
```

1. Создание сессии и получение QR-кода

```bash
grpcurl -plaintext -d '{}' localhost:50051 pact.telegram.TelegramService/CreateSession
```

Ответ:

```json
{
  "sessionId": "550e8400-e29b-41d4-a716-446655440000",
  "qrCode": "tg://login?token=5a8f1d3e9b2c7a4d6f8e0c1b3a5d7f9e2c4a6b8d"
}
```

Процесс авторизации:

1. Сгенерируйте QR-код из полученной ссылки (например, на https://qrcode.tec-it.com)
2. Откройте Telegram на телефоне
3. Перейдите в Settings → Devices → Scan QR
4. Отсканируйте QR-код

2. Отправка сообщения

```bash
grpcurl -plaintext -d '{
  "sessionId": "550e8400-e29b-41d4-a716-446655440000",
  "peer": "@durov",
  "text": "Hello from Telegram Service!"
}' localhost:50051 pact.telegram.TelegramService/SendMessage
```

Ответ:

```json
{
  "messageId": 123456789
}
```

3. Подписка на входящие сообщения

```bash
grpcurl -plaintext -d '{
  "sessionId": "550e8400-e29b-41d4-a716-446655440000"
}' localhost:50051 pact.telegram.TelegramService/SubscribeMessages
```

При получении сообщения:

```json
{
  "messageId": 123456790,
  "from": "@username",
  "text": "Привет!",
  "timestamp": 1741104000
}
```

4. Удаление сессии

```bash
grpcurl -plaintext -d '{
  "sessionId": "550e8400-e29b-41d4-a716-446655440000"
}' localhost:50051 pact.telegram.TelegramService/DeleteSession
```

Ответ:

```json
{}
```

Пример клиента на Go

```go
package main

import (
    "context"
    "log"
    "time"

    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"

    pb "telegram_project/asd/api/grpc/proto"
)

func main() {
    conn, err := grpc.Dial("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()

    client := pb.NewTelegramServiceClient(conn)

    // Создание сессии
    resp, err := client.CreateSession(context.Background(), &pb.CreateSessionRequest{})
    if err != nil {
        log.Fatal(err)
    }
    log.Printf("Session ID: %s", *resp.SessionId)
    log.Printf("QR Code: %s", *resp.QrCode)

    // Ожидание авторизации
    time.Sleep(30 * time.Second)

    // Отправка сообщения
    sendResp, err := client.SendMessage(context.Background(), &pb.SendMessageRequest{
        SessionId: resp.SessionId,
        Peer:      stringPtr("@durov"),
        Text:      stringPtr("Hello!"),
    })
    if err != nil {
        log.Fatal(err)
    }
    log.Printf("Message sent, ID: %d", *sendResp.MessageId)
}

func stringPtr(s string) *string { return &s }
```

🏗 Описание архитектурных решений

1. Изоляция сессий

Каждая сессия работает в отдельной горутине с собственным:
22:06
· Контекстом (context.WithCancel)
· Клиентом Telegram (telegram.Client)
· Обработчиком обновлений

Преимущество: При падении одной сессии остальные продолжают работу.

2. Потокобезопасность

· Доступ к карте сессий защищён sync.RWMutex
· Рассылка сообщений через буферизированные каналы (buffer: 10)
· Копирование слайсов подписчиков для избежания блокировок

3. Авторизация через QR

В соответствии с ТЗ реализована следующим образом:

1. Запрос AuthExportLoginToken к Telegram API
2. Формирование ссылки tg://login?token=...
3. Возврат ссылки клиенту для генерации QR
4. Периодическая проверка статуса авторизации (каждые 2 сек)
5. После сканирования - запуск обработчика обновлений

4. Получение сообщений

· Используется updates.Manager из библиотеки gotd/td
· Обработчик OnNewMessage получает все входящие сообщения
· Рассылка подписчикам через каналы с таймаутом (100ms)
· Поддержка множества одновременных подписок

5. Удаление сессии

· Вызов AuthLogOut (если сессия была авторизована)
· Отмена контекста через cancel()
· Ожидание завершения горутины через канал done
· Удаление из карты менеджера

6. Обработка ошибок

· Все ошибки логируются через zap.Logger
· gRPC методы возвращают соответствующие статусы (codes.Internal, codes.NotFound)
· Паника одной сессии не влияет на другие

⚠️  Возможные проблемы и решения

"session not ready"

Причина: QR-код ещё не отсканирован
Решение: Подождите 30 секунд или отсканируйте QR

"peer not found"

Причина: Неправильный username
Решение: Проверьте написание (можно с @ или без)

"failed to export login token"

Причина: Неверные APP_ID или APP_HASH
Решение: Проверьте credentials на my.telegram.org

Сессия не получает сообщения

Причина: Не запущен updates manager
Решение: Пересоздайте сессию и проверьте логи
