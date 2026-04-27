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

---

## Что делает агент

| Подсистема | Что |
|---|---|
| **Traps** | TCP-ловушки на случайных high-портах (10000–60000): SSH-баннер, HTTP «Apache», FTP USER/PASS, MySQL handshake. Любой коннект → событие. |
| **File canary** | inotify на чувствительные файлы (`.env`, `id_rsa`, `/etc/shadow`) + автоматически создаваемые приманки в `/root`, `/tmp`, `/var/www`. |
| **Correlator** | Группирует события по IP в окне 30 минут. Считает 0–100 score: количество событий, разнообразие типов, file canary, плотность во времени, известные паттерны (`ssh→canary`, `http→db→canary`). |
| **Firewall** | iptables-цепочка `GORONIN-BLOCK`, threshold-based блокировка с эскалацией (3 хита/5 мин → бан 1 ч, повтор → 24 ч). Persistent — переживает reboot. |
| **AI** | Claude / GPT / Gemini пишет короткий разбор события и цепочки. Опционально. |
| **Telegram** | Per-event alert + chain alert при score ≥ 50 + startup notification. |

---

## Архитектура

```
        Сервер клиента
┌──────────────────────────────────────────┐
│  goronin (Go, ~10 МБ, < 30 МБ RAM)       │
│                                          │
│  ┌─────────┐   ┌────────────┐            │
│  │ Traps   │──▶│            │            │
│  └─────────┘   │            │   ┌─────┐  │
│  ┌─────────┐   │ Correlator │──▶│ AI  │  │
│  │ Watcher │──▶│            │   └──┬──┘  │
│  └─────────┘   │            │      │     │
│       │        └─────┬──────┘      │     │
│       │              │             ▼     │
│  ┌────▼──────┐  ┌────▼─────┐  ┌────────┐ │
│  │ Firewall  │◀─│ Alerter  │─▶│Telegram│─┼──▶ HTTPS
│  │ (iptables)│  └──────────┘  └────────┘ │
│  └────┬──────┘                           │
│       │                                  │
│  ┌────▼─────────────┐                    │
│  │ bbolt /var/lib   │                    │
│  │ (hits + blocks)  │                    │
│  └──────────────────┘                    │
└──────────────────────────────────────────┘
```

Никакого центрального сервера. Все данные локальны. Исходящий трафик — только в Telegram API и (опционально) в API выбранного AI-провайдера.

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

whitelist_ips:
  - 203.0.113.42             # твой IP
  - 10.0.0.0/8

watch_files:                 # дополнительно к авто-обнаружению
  - /var/www/secrets.json

data_dir: /var/lib/goronin
```

После правки руками: `sudo goronin restart`.

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
│       ├── ai/             # Anthropic / OpenAI / Gemini
│       ├── alerter/        # роутинг events → AI → Telegram
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

- [x] Standalone-агент без бэкенда
- [x] 3 AI-провайдера (Anthropic / OpenAI / Gemini)
- [x] Persistent firewall blocks через bbolt
- [x] Threshold-based auto-ban + alert_only mode
- [x] Interactive install wizard
- [x] systemd integration + CLI обёртки
- [ ] Real SSH honeypot на `golang.org/x/crypto/ssh` (логирование пар user/pass)
- [ ] GitHub Actions: cross-compile + auto-release бинарей
- [ ] Поддержка nftables (для дистрибутивов без iptables)

---

## Лицензия

MIT — см. [LICENSE](LICENSE).
