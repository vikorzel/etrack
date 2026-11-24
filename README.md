# Meal Satiety Telegram Bot (Go)

Простой Telegram-бот на Go для трекинга сытости до и после приёма пищи с выгрузкой данных в CSV за выбранный период.

## Возможности

- **🍽 Записать приём пищи** — пошаговый опрос кнопками от `1` до `10`:
  1. Сытость **до** приёма пищи
  2. Сытость **после** приёма пищи (с кнопками `Назад` и `Отмена`)
- **📊 Выгрузить CSV** — экспорт записей за период:
  - пресеты: `За 7 дней`, `За 30 дней`, `За всё время`
  - `📅 Свой период` в формате `DD.MM.YYYY - DD.MM.YYYY`, `YYYY-MM-DD - YYYY-MM-DD` или одна дата (`24.11.2026`)
- Хранение в локальной базе SQLite (`modernc.org/sqlite`, pure Go, без CGO) с изоляцией данных по `user_id`.

## Настройка

Бот поддерживает два режима загрузки конфигурации:
1. **PocketBase** (если задана переменная `PB_CONFIG_HOST`) — весь конфиг, включая `telegram_bot_token`, забирается из PocketBase без авторизации (для внутренней сети). Локальные переменные при этом не используются (ошибка или отсутствие записи в PocketBase прерывает старт).
2. **Локальный режим** (если `PB_CONFIG_HOST` не задан) — параметры берутся из переменных окружения или файла `.env`.

### Режим 1: Конфигурация из PocketBase

Создайте в PocketBase **Base-коллекцию** `configs` (или укажите своё имя в `PB_CONFIG_COLLECTION`) со следующими полями:

| Поле | Тип | Обязательно | Описание |
| --- | --- | --- | --- |
| `app` | `Text` (Unique) | Да | Ключ сервиса, по умолчанию `meal-bot` |
| `telegram_bot_token` | `Text` | Да | Токен Telegram-бота от `@BotFather` |
| `db_path` | `Text` | Нет | Путь к SQLite-файлу (по умолчанию `meals.db`) |
| `bot_timezone` | `Text` | Нет | IANA-таймзона (например `Europe/Moscow`, по умолчанию локальная ОС) |

Рекомендуемые API-правила коллекции для внутренней сети:
- `List/Search` и `View`: пустая строка `""` (открытое чтение внутри приватной сети)
- `Create`, `Update`, `Delete`: только суперпользователи (оставить `null` / superusers only)

Пример записи в коллекции `configs`:

```json
{
  "app": "meal-bot",
  "telegram_bot_token": "123456789:replace-with-your-bot-token",
  "db_path": "meals.db",
  "bot_timezone": "Europe/Moscow"
}
```

Переменные окружения для режима PocketBase:

| Переменная | Обязательна | По умолчанию | Описание |
| --- | --- | --- | --- |
| `PB_CONFIG_HOST` | Да (для PB) | — | Базовый URL PocketBase, например `http://pocketbase:8090` или `pocketbase:8090` |
| `PB_CONFIG_COLLECTION` | Нет | `configs` | Имя коллекции в PocketBase |
| `PB_CONFIG_APP` | Нет | `meal-bot` | Значение поля `app` для фильтрации записи |

Проверка доступности записи через `curl`:

```bash
curl "http://pocketbase:8090/api/collections/configs/records?filter=(app='meal-bot')&perPage=1"
```

### Режим 2: Локальный fallback (`.env` / переменные окружения)

Если `PB_CONFIG_HOST` не задан:
1. Создайте бота через `@BotFather` в Telegram и получите токен.
2. Скопируйте `.env.example` в `.env` и заполните `TELEGRAM_BOT_TOKEN`:

```bash
cp .env.example .env
```

Доступные переменные окружения для локального режима:

| Переменная | Обязательна | По умолчанию | Описание |
| --- | --- | --- | --- |
| `TELEGRAM_BOT_TOKEN` | Да | — | Токен Telegram-бота |
| `DB_PATH` | Нет | `meals.db` | Путь к файлу SQLite |
| `BOT_TIMEZONE` | Нет | Локальный пояс ОС | IANA-таймзона (например `Europe/Moscow`, `UTC`) |

## Запуск и сборка

```bash
PB_CONFIG_HOST=http://pocketbase:8090 go run ./cmd/bot
```

Сборка бинарника:

```bash
go build -o meal-bot ./cmd/bot
```

> **Примечание для Windows + WSL (`\\wsl.localhost\...`)**: Windows-версия `go.exe` пытается получить файловую блокировку `LockFileEx` (`RLock`) на `go.mod` даже в режиме `-mod=readonly`, а файловая система 9P в WSL (`\\wsl.localhost\...`) её не поддерживает (`Incorrect function.`). Чтобы запускать команды `go` в `~/code/meal-bot`, установите Go внутри WSL Ubuntu (например, через `go.dev/dl` или пакетный менеджер) и запускайте из терминала WSL, либо перенесите/скопируйте папку проекта на локальный диск Windows (NTFS) при использовании Windows `go.exe`.

## Тесты

```bash
go test -v ./...
```

## Формат CSV

Экспортируемый файл содержит колонки:

```text
id,date,time,recorded_at,satiety_before,satiety_after,delta
```

- `date`, `time`, `recorded_at` приводятся к настроенному часовому поясу (`BOT_TIMEZONE` / `bot_timezone` или локальному).
- `delta` = `satiety_after - satiety_before`.
