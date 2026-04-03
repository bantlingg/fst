# Go REST API + Nginx + Статика

Простое демонстрационное приложение:

- **Backend**: Go (`net/http`)
- **Хранение**: сообщения в файле `messages.json`
- **Endpoints**:
  - `POST /message` — принимает JSON `{ "text": "..." }`, сохраняет в файл
  - `GET /message` — возвращает список всех сообщений
- **Frontend**: минимальная страница `static/index.html` с `fetch` к API
- **Nginx**: пример конфигурации в `nginx.conf.example`

## Запуск Go-сервера

Из корня проекта:

```bash
go run ./...
```

Сервер по умолчанию слушает `:8080`.

## Формат сообщений

Файл `messages.json` содержит массив объектов:

```json
[
  {
    "text": "Привет",
    "timestamp": "2026-04-03T12:34:56Z"
  }
]
```

## Эндпоинты

- **POST /message**
  - Тело запроса (JSON):

    ```json
    { "text": "Моё сообщение" }
    ```

  - Успешный ответ (`201`):

    ```json
    {
      "status": "ok",
      "message": {
        "text": "Моё сообщение",
        "timestamp": "2026-04-03T12:34:56Z"
      }
    }
    ```

- **GET /message**
  - Ответ (`200`): JSON-массив всех сообщений.

## Настройка Nginx (статические файлы)

Файл-пример: `nginx.conf.example`.

Минимальные шаги (Windows, стандартный nginx):

1. Скопируйте/обновите `nginx.conf` в папке установки Nginx, используя пример из `nginx.conf.example`.
2. В секции `server` укажите путь к статикам:

   ```nginx
   root  C:/Users/Nikita/Downloads/goapp/static;
   ```

3. Запустите/перезапустите Nginx.

После этого:

- Nginx раздаёт `index.html` по `http://localhost/`
- Go-сервер обслуживает API по `http://localhost:8080/message`

Если хотите, чтобы запросы `/message` шли через Nginx (reverse proxy), раскомментируйте пример сервера в `nginx.conf.example` и настройте `proxy_pass` на `http://127.0.0.1:8080`.

