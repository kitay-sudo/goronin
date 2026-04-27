# Changelog

Формат — [Keep a Changelog](https://keepachangelog.com/ru/1.1.0/), версионирование — [SemVer](https://semver.org/lang/ru/).

## [Unreleased]

### Planned

- Real SSH honeypot на `golang.org/x/crypto/ssh` — логирование пар `(username, password)` и публичных ключей.
- Поддержка nftables (для дистрибутивов где iptables помечен deprecated).
- e2e-проверка install.sh в CI (раз в неделю прогон curl|bash в Docker).

## [0.2.0] — 2026-04-28

Двухуровневая агрегация алертов. На скан в 30 коннектов теперь 1 alert + 1 AI-вызов вместо 30 + 30. Подробнее в новом документе [HOW_IT_WORKS.md](HOW_IT_WORKS.md).

### Added

- **Пакет `internal/aggregator`** с двумя «корзинами»:
  - **Срочная** (5 мин по умолчанию) — копит все события, на закрытии группирует по IP, считает суммарный score, отправляет один сводный alert с AI-разбором (если score ≥ 30).
  - **Фоновая** (1 час по умолчанию) — принимает «недотянувшие» сводки из срочной, раз в час шлёт компактный дайджест без AI.
- **AI-провайдеры получили `AnalyzeBatch`** — один промпт на всю агрегированную сводку вместо одного на каждое событие. Новый системный промпт под формат "несколько IP за окно".
- **Новые Telegram-форматтеры**: `FormatBatchAlert` (срочная сводка с группировкой по IP и эмодзи-маркерами по уровню угрозы), `FormatBackgroundDigest` (часовой компактный дайджест с топ-5 IP).
- **`AlertingConfig` в config.yml** — три тунабла (`urgent_window`, `background_window`, `interest_threshold`). Дефолты разумные, можно не трогать.
- **Мгновенная ветка для file canary write/remove** — обходит агрегатор, AI-разбор и алерт приходят сразу. Read остаётся через 5-мин окно (защита от ложных срабатываний от cron/backup).
- **`HOW_IT_WORKS.md`** в корне — пояснительный документ для пользователей: как работают уведомления, во сколько обходится AI, что приходит в Telegram при разной нагрузке.

### Changed

- **`alerter` переписан под sweep-модель.** Старый метод `Handle(event)` больше не существует; вместо него `FlushBatch(batch)` (для агрегатора) и `HandleInstant(event)` (для file canary). Сигнатура конструктора упрощена: больше не принимает correlator.
- **`correlator.calculateScore` экспортирован как `CalculateScore`** — теперь используется и из агрегатора, и из старого correlator (chain-логика для будущего chain-alerta).
- **`main.go`**: вместо прямой `al.Handle(event)` теперь events идут через `agg.Observe(event)` (или `al.HandleInstant` для file canary).

### Performance / cost

- **AI-вызовов в сутки на типичном сервере: было 200+, стало 5-10.**
- **Telegram-сообщений в сутки: было 200+, стало 5-15** (см. HOW_IT_WORKS.md → "Что будет на разных серверах").
- На gpt-4o-mini расход AI: $0.005-0.02/день вместо $0.30-0.90/день.

## [0.1.0] — 2026-04-27

Первый open-source релиз. GORONIN перерождён как полностью standalone-агент: один Go-бинарь, никакого центрального бэкенда.

### Added
- **Standalone-агент** — все операции (ловушки, корреляция, AI-разбор, Telegram-алерты, авто-бан) выполняются локально на сервере клиента.
- **3 AI-провайдера на выбор**: Anthropic Claude, OpenAI GPT, Google Gemini. Опционально — без AI агент тоже работает, просто без объяснительного абзаца в алертах.
- **Интерактивный wizard** (`goronin install`) — спрашивает Telegram bot/chat, AI-провайдер и ключ, какие ловушки включить, режим авто-бана, whitelist IP. Проверяет Telegram через `getMe` + тест-сообщение.
- **Persistent state через bbolt** (`/var/lib/goronin/state.db`) — счётчики хитов и активные баны переживают reboot и рестарт сервиса.
- **Threshold-based авто-бан** с режимами `off` / `alert_only` / `enforce` и эскалацией (3 хита/5 мин → бан 1ч, повтор → 24ч).
- **systemd-интеграция** — wizard сам регистрирует unit-файл, делает `enable + start`. CLI-обёртки `goronin start | stop | restart | status | logs [-f] | unban <ip> | reset | reconfigure | version`.
- **One-command installer** (`install.sh`) — определяет архитектуру (amd64/arm64), скачивает релизный бинарь из GitHub, кладёт в `/usr/local/bin`, запускает wizard.
- **Новый лендинг** под open-source позиционирование: copy-кнопка для curl-команды, ссылки на GitHub, без страниц кабинета.

### Changed
- **Архитектура: SaaS → standalone open-source.** Удалены Node.js/Fastify бэкенд, PostgreSQL, JWT-аутентификация, регистрация серверов, dashboard. Логика AI-анализа, корреляции цепочек атак и Telegram-форматирования вынесена из бэкенда в Go-агент.
- **Firewall теперь persistent.** Раньше `Shutdown()` флашил iptables-цепочку — баны терялись при рестарте. Теперь блоки записываются в bbolt и восстанавливаются на старте через `RestoreFromStorage()`.
- **Module path:** `github.com/goronin-io/agent` → `github.com/kitay-sudo/goronin/agent`.

### Removed
- `backend/` — целиком (Node.js + Fastify + Postgres).
- `agent/internal/{client,heartbeat,fingerprint}` — backend-специфичные пакеты.
- `frontend/src/pages/{Login,Onboarding,Dashboard,ServerDetail}.jsx`, `AuthContext`, `lib/api.js`, `lib/router.js` — кабинет больше не нужен.
- `docker-compose.yml`, `Dockerfile`, `deploy.sh`, `errors.sh`, старый `install.sh` (под backend-деплой).
- Биллинг, тарифы, регистрация — продукт стал бесплатным opensource без подписок.

### Security
- Конфиг (`/etc/goronin/config.yml`) и state (`/var/lib/goronin/state.db`) пишутся с `mode 0600`, owner `root`.
- Все секреты (Telegram bot token, AI API key) живут только на машине пользователя — нет центрального сервиса, который мог бы их утечь.
- Telegram-сообщения проходят через `htmlEscape` для всех user-controlled полей (IP, имена файлов, AI-вывод).

[Unreleased]: https://github.com/kitay-sudo/goronin/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/kitay-sudo/goronin/releases/tag/v0.1.0
