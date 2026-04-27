<p align="center">
  <h1 align="center">GORONIN</h1>
  <p align="center"><strong>Open-source honeypot guard. Один бинарь. Ноль бэкенда.</strong></p>
  <p align="center"><em>浪人 — Страж без хозяина. Молча ждёт. Вовремя бьёт.</em></p>
</p>

<p align="center">
  <img src="https://img.shields.io/badge/agent-Go-00ADD8?logo=go&logoColor=white" alt="Go">
  <img src="https://img.shields.io/badge/AI-Claude%20%7C%20OpenAI%20%7C%20Gemini-7C3AED" alt="AI">
  <img src="https://img.shields.io/badge/alerts-Telegram-26A5E4?logo=telegram&logoColor=white" alt="Telegram">
  <img src="https://img.shields.io/badge/runtime-systemd-FCC624?logo=linux&logoColor=black" alt="systemd">
  <img src="https://img.shields.io/badge/license-MIT-blue" alt="MIT">
</p>

---

## Установка

```bash
curl -sSL https://raw.githubusercontent.com/kitay-sudo/goronin/main/install.sh | sudo bash
```

Скрипт скачает бинарь под твою архитектуру (amd64 / arm64), положит его в `/usr/local/bin/goronin` и запустит интерактивный wizard. Wizard спросит:

- Telegram bot token + chat_id (с проверкой через `getMe` и тест-сообщением)
- AI-провайдер: Anthropic / OpenAI / Gemini / none (опционально)
- Какие ловушки включить (SSH / HTTP / FTP / MySQL)
- Режим авто-бана (off / alert_only / enforce) + threshold
- Whitelist IP

Через минуту в Telegram придёт сообщение «GORONIN запущен» с перечнем активных ловушек.

> 📖 **Хочешь понять как именно работают уведомления, во сколько обходится AI и что приходит в Telegram?** → [HOW_IT_WORKS.md](HOW_IT_WORKS.md) — пояснительный документ для пользователей, на простом языке без терминов.

---

## Что делает агент

| Подсистема | Что |
|---|---|
| **Traps** | TCP-ловушки на случайных high-портах (10000–60000): SSH-баннер, HTTP «Apache», FTP USER/PASS, MySQL handshake. Любой коннект → событие. |
| **File canary** | inotify на чувствительные файлы (`.env`, `id_rsa`, `/etc/shadow`) + автоматически создаваемые приманки в `/root`, `/tmp`, `/var/www`. |
| **Correlator** | Группирует события по IP в окне 30 минут. Считает 0–100 score: количество событий, разнообразие типов, file canary, плотность во времени, известные паттерны (`ssh→canary`, `http→db→canary`). |
| **Firewall** | iptables-цепочка `GORONIN-BLOCK`, threshold-based блокировка с эскалацией (3 хита/5 мин → бан 1 ч, повтор → 24 ч). Persistent — переживает reboot. |
| **AI** | Claude / GPT / Gemini пишет короткий разбор события и цепочки. Опционально. |
| **Aggregator** | Двухуровневое окно: 5 мин «срочное» + 1 час «фоновое». События с одного сервера копятся, отправляются одной сводкой по всем IP. AI вызывается только когда суммарная угроза ≥ 30. Файловые ловушки на запись/удаление идут мимо агрегатора — мгновенный alert. Подробнее в [HOW_IT_WORKS.md](HOW_IT_WORKS.md). |
| **Telegram** | Один сводный alert за окно вместо потока. Фоновый дайджест раз в час без AI. Мгновенный alert на file canary write. |

---

## Архитектура

Один процесс на сервере, ~10 МБ бинарь, ~30 МБ RAM в простое.

**Поток события:** ловушка или watcher замечает активность → событие попадает в firewall (счётчик хитов растёт, при превышении threshold — бан в iptables) и в correlator (группирует по IP, считает score). Дальше alerter формирует сообщение, при необходимости запрашивает разбор у AI и шлёт в Telegram.

**Состояние** (счётчики хитов, активные баны) хранится локально в bbolt (`/var/lib/goronin/state.db`) и переживает reboot.

**Никакого центрального сервера.** Все данные на твоей машине. Исходящий трафик — только в Telegram Bot API и (опционально) в API выбранного AI-провайдера. Входящих соединений к агенту нет.

---

## CLI

После установки доступны:

```bash
goronin status              # systemctl status
goronin logs -f             # journalctl -u goronin -f
goronin restart             # перезапустить демон
goronin stop / start
goronin unban 1.2.3.4       # снять бан вручную
goronin reset               # сбросить ВСЕ баны и очистить iptables
goronin reconfigure         # перезапустить wizard, сохранить новый конфиг, рестарт
goronin version
```

Все команды требуют root (используют systemctl и iptables).

---

## Конфиг

`/etc/goronin/config.yml` (mode 0600). Создаётся wizard'ом, можно править руками.

```yaml
server_name: prod-web-01

telegram:
  bot_token: "1234567890:ABCdefGHI"
  chat_id: "555000111"

ai:
  provider: anthropic        # anthropic | openai | gemini | "" (отключено)
  api_key: "sk-ant-..."
  model: "claude-sonnet-4-6" # опционально, есть дефолт на провайдер

traps:
  ssh: true
  http: true
  ftp: true
  db: true

auto_ban:
  mode: enforce              # off | alert_only | enforce
  threshold: 3               # хитов до бана
  window: 5m                 # окно подсчёта
  block_duration: 1h         # длительность первого бана (повтор → 24ч)

alerting:
  urgent_window: 5m          # окно «срочной» агрегации
  background_window: 1h      # окно «фонового» дайджеста
  interest_threshold: 30     # суммарный score ниже которого batch уходит в фон

whitelist_ips:
  - 203.0.113.42             # твой IP
  - 10.0.0.0/8

watch_files:                 # дополнительно к авто-обнаружению
  - /var/www/secrets.json

data_dir: /var/lib/goronin
```

После правки руками: `sudo goronin restart`.

---

## Как изменить настройки уже после установки

Три способа в порядке удобства.

### 1. Поправить YAML руками (для точечных изменений)

```bash
sudo nano /etc/goronin/config.yml      # или vim, code -- что есть
sudo goronin restart                   # применить
```

Подходит когда нужно поменять одну-две строчки. Примеры:

**Выключить AI совсем** (алерты будут приходить, просто без объяснительных абзацев):

```yaml
ai:
  provider: ""        # пустая строка = AI отключён
  api_key: ""
```

**Выключить авто-бан** (только мониторинг, без блокировки в iptables):

```yaml
auto_ban:
  mode: "off"         # off | alert_only | enforce
```

**Сменить AI-провайдера** (например с Claude на бесплатный Gemini):

```yaml
ai:
  provider: gemini
  api_key: "g-key-..."
  model: "gemini-2.0-flash"
```

**Реже сводки в Telegram** (увеличить окно агрегации с 5 до 15 минут):

```yaml
alerting:
  urgent_window: 15m
```

**Добавить IP в whitelist** (например IP мониторинга или нового пентестера):

```yaml
whitelist_ips:
  - 203.0.113.42
  - 198.51.100.10    # новый IP
```

После правки **всегда** делай `sudo goronin restart`. До рестарта изменения не применятся.

### 2. Заново пройти wizard (для крупных изменений)

```bash
sudo goronin reconfigure
```

Запустит интерактивный мастер заново. На каждом вопросе текущее значение подставлено как default — Enter оставляет как есть, ввод нового значения перезаписывает. Полезно если меняешь несколько настроек сразу или хочешь сменить Telegram-бота.

После завершения сервис перезапускается автоматически.

### 3. Временно остановить агента (без правки конфига)

```bash
sudo goronin stop                       # остановить сейчас
sudo systemctl disable goronin          # не запускать после reboot
```

Полезно для maintenance — например когда сам пентестишь свой сервер и не хочешь засорять алерты, или когда меняешь сетевые настройки. Чтобы вернуть:

```bash
sudo systemctl enable goronin
sudo goronin start
```

### Как проверить что новые настройки применились

```bash
sudo goronin status                     # сервис активен и работает
sudo goronin logs -f                    # видно что запустилось без ошибок
```

В Telegram сразу после рестарта приходит startup-сообщение «GORONIN запущен» с актуальным списком ловушек — это и есть подтверждение что новый конфиг подхватился.

---

## Безопасность и прозрачность

- Лицензия MIT. Полный код открыт. Бинарь воспроизводимо собирается из `cmd/goronin`.
- Никаких бэкендов — все ключи и события живут на твоём сервере.
- Конфиг и state.db — mode 0600, owner root.
- Агент **не**: читает содержимое произвольных файлов клиента, перехватывает трафик, отправляет данные с сервера, имеет доступ к продуктивным БД.
- Агент шлёт наружу: тип события + IP + порт + время + (опционально) AI-промпт с теми же данными.

---

## Сборка из исходников

```bash
git clone https://github.com/kitay-sudo/goronin.git
cd goronin/agent
go build -ldflags "-X main.version=$(git describe --tags --always)" -o goronin ./cmd/goronin
sudo install -m 0755 goronin /usr/local/bin/
sudo /usr/local/bin/goronin install
```

Cross-compile под ARM-сервер с x86 машины:

```bash
GOOS=linux GOARCH=arm64 go build -o goronin-linux-arm64 ./cmd/goronin
```

---

## Тесты

```bash
cd agent
go vet ./...
go test ./... -v
```

Покрытие: storage, ai (mock-серверы для всех трёх провайдеров), telegram, correlator, firewall (in-memory iptables-mock + persistent storage), traps (live-listeners), config.

---

## Структура

```
goronin/
├── install.sh              # one-command installer
├── README.md
├── LICENSE                 # MIT
│
├── agent/                  # Go-бинарь — это весь продукт
│   ├── cmd/goronin/        # entry + CLI subcommands
│   ├── pkg/protocol/       # EventRequest и константы
│   └── internal/
│       ├── aggregator/     # 5-мин/1-час двухуровневое окно для батчинга алертов
│       ├── ai/             # Anthropic / OpenAI / Gemini
│       ├── alerter/        # sweep events → AI → Telegram
│       ├── config/         # YAML config + validation
│       ├── correlator/     # chain analysis + scoring
│       ├── firewall/       # iptables + persistent blocks
│       ├── storage/        # bbolt wrapper
│       ├── systemd/        # unit-file generation + start/stop wrappers
│       ├── telegram/       # bot client + message formatters
│       ├── traps/          # SSH, HTTP, FTP, MySQL listeners
│       ├── watcher/        # inotify + canary creation
│       └── wizard/         # interactive install wizard
│
├── frontend/               # статический лендинг (отдельно деплоится на домен)
│   └── src/pages/Landing.jsx
│
└── docs/
    ├── INSTALL.md
    ├── CONFIG.md
    └── ARCHITECTURE.md
```

---

## Roadmap

### v0.2.0 — текущий релиз

- [x] **Двухуровневая агрегация алертов.** 5-минутное «срочное» окно собирает события со всех IP, отправляет одну сводку. Низкоскоровые сводки уходят в часовой «фоновый» дайджест без AI. На скан в 30 коннектов теперь 1 alert + 1 AI-вызов вместо 30 + 30. Подробнее в [HOW_IT_WORKS.md](HOW_IT_WORKS.md).
- [x] Файловая ловушка на write/remove — мгновенный alert (минуя агрегатор).
- [x] AnalyzeBatch на стороне AI — один промпт на всю сводку, не на каждое событие.

### v0.1.0

- [x] Standalone-агент без бэкенда
- [x] 3 AI-провайдера (Anthropic / OpenAI / Gemini)
- [x] Persistent firewall blocks через bbolt
- [x] Threshold-based auto-ban (off / alert_only / enforce)
- [x] Interactive install wizard
- [x] systemd integration + CLI обёртки (start/stop/restart/logs/unban/reset)
- [x] Cross-compile в CI: linux/amd64 + linux/arm64 release-бинари

### Дальше

- [ ] **Real SSH honeypot на `golang.org/x/crypto/ssh`.** Сейчас SSH-trap отдаёт баннер и закрывает коннект — мы не видим что пытался ввести бот. С crypto/ssh будем логировать пары `(username, password)` и публичные ключи.
- [ ] **Поддержка nftables.** Для новых дистрибутивов (Fedora 40+, RHEL 9+, Debian 12+) где iptables помечен deprecated.
- [ ] **e2e-проверка install.sh в CI.** Раз в неделю прогонять `curl ... | bash` в Docker — страховка чтобы установка не сломалась незаметно.
- [ ] **Релиз-бинари для macOS/Windows.** Для локальной разработки и тестов перед прод-установкой (без systemd, в foreground-режиме).

---

## Лицензия

MIT — см. [LICENSE](LICENSE).
