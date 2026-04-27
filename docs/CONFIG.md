# Конфигурация GORONIN

Файл: `/etc/goronin/config.yml`, mode `0600`, owner `root`.

Создаётся wizard'ом при `goronin install`. Можно менять руками — после правки выполни `sudo goronin restart`.

## Полный пример

```yaml
server_name: prod-web-01

telegram:
  bot_token: "1234567890:AAEhBOOO..."
  chat_id: "555000111"

ai:
  provider: anthropic
  api_key: "sk-ant-..."
  model: "claude-sonnet-4-6"

traps:
  ssh: true
  http: true
  ftp: true
  db: true

auto_ban:
  mode: enforce
  threshold: 3
  window: 5m
  block_duration: 1h

whitelist_ips:
  - 203.0.113.42
  - 10.0.0.0/8

watch_files:
  - /var/www/secrets.json

data_dir: /var/lib/goronin
```

## Поля

### `server_name` (string)

Что будет в заголовке Telegram-алертов. По умолчанию — hostname.

### `telegram` (обязательно)

| Поле | Тип | Описание |
|---|---|---|
| `bot_token` | string | Токен бота от `@BotFather` |
| `chat_id` | string | Числовой ID чата (для групп — с минусом) |

Без telegram-блока агент не запустится — это единственный канал доставки.

### `ai` (опционально)

| Поле | Тип | Описание |
|---|---|---|
| `provider` | enum | `anthropic` / `openai` / `gemini` / `""` (отключено) |
| `api_key` | string | Ключ API. Обязателен, если provider не пуст |
| `model` | string | Модель. Если не указано — дефолт по провайдеру (см. ниже) |

Дефолтные модели:
- `anthropic` → `claude-sonnet-4-6`
- `openai` → `gpt-4o-mini`
- `gemini` → `gemini-2.0-flash`

Без AI алерты приходят, просто без объяснительного абзаца.

### `traps` (обязательно)

Какие ловушки запускать. Каждая занимает один случайный high-порт (10000–60000) при старте.

| Поле | Что эмулирует |
|---|---|
| `ssh` | OpenSSH-баннер, читает client banner |
| `http` | Apache-like ответ, логирует method/path/UA |
| `ftp` | vsFTPd USER/PASS handshake |
| `db` | MySQL handshake |

Можно отключить любые — например, если на сервере уже стоит prod-MySQL и не хочется лишней путаницы.

### `auto_ban`

| Поле | Тип | Дефолт | Описание |
|---|---|---|---|
| `mode` | enum | `enforce` | `off` (не банить), `alert_only` (логировать но не банить — dry-run), `enforce` (банить) |
| `threshold` | int | `3` | Сколько коннектов от одного IP до бана |
| `window` | duration | `5m` | Окно подсчёта хитов |
| `block_duration` | duration | `1h` | Длительность первого бана. Повторное нарушение → 24h |

`duration` — Go-формат: `30s`, `5m`, `1h`, `24h`.

### `whitelist_ips`

Список IP/CIDR, которые **никогда** не банятся. Сюда автоматически добавляются `127.0.0.1`, `::1`, `localhost`. Wizard предлагает добавить твой текущий outbound IP (определяется через `udp 8.8.8.8:80`).

Сюда же стоит добавить:
- IP пентестеров и red-team
- IP мониторинга (Zabbix, Prometheus, UptimeRobot)
- IP CI/CD (GitHub Actions, GitLab runners)

### `watch_files`

Дополнительные пути для inotify-мониторинга. К ним добавляется auto-discovery (`/root/.env`, `/home/*/.ssh/id_rsa`, `/etc/shadow` и т.п.) и созданные канарейки в `/root`, `/tmp`, `/var/www`.

### `data_dir`

Где хранится `state.db` (bbolt: hits, blocks, meta). Дефолт `/var/lib/goronin`.

## Hot-reload

Сейчас не поддерживается. После правки конфига:

```bash
sudo goronin restart
```

При перезапуске:
- Активные баны сохраняются (читаются из `state.db`)
- Hit-счётчики сохраняются
- Ловушки получают новые случайные порты
- В Telegram приходит startup-сообщение

## Скрытие секретов

Конфиг содержит чувствительные данные (Telegram bot token, AI API key). Файл создаётся с правами `0600`, владелец root. Не коммить его в git.

Если секреты утекли:
1. Перевыпусти токен бота через `@BotFather` (`/revoke` → `/newbot`)
2. Перевыпусти AI API key в консоли провайдера
3. `sudo goronin reconfigure` — заново пройди wizard с новыми ключами

## Проверка валидности

```bash
sudo goronin daemon
```

(не запускай так на проде — это foreground-режим). Если конфиг невалиден, выдаст ошибку и выйдет. На проде эту проверку делает systemd при `restart`.
