# Changelog

Формат — [Keep a Changelog](https://keepachangelog.com/ru/1.1.0/), версионирование — [SemVer](https://semver.org/lang/ru/).

## [Unreleased]

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
