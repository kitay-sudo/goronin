# Архитектура GORONIN

Standalone-агент. Один процесс, никакого центрального сервера. Всё state — на машине.

## Поток событий

```
                         ┌──────────────┐
                         │   onEvent    │ (callback в main.go)
                         └──────┬───────┘
                                │
              ┌─────────────────┼─────────────────┐
              ▼                 ▼                 ▼
       ┌──────────┐      ┌────────────┐     ┌─────────┐
       │ firewall │◀──── │ correlator │     │   ai    │
       │.RecordHit│      │  .Observe  │     │.Analyze │
       └────┬─────┘      └──────┬─────┘     └────┬────┘
            │                   │                │
            ▼                   ▼                ▼
       iptables            in-memory chain   Telegram
       + bbolt            (per source IP)
```

1. **Trap** или **watcher** генерирует `EventRequest` (тип, IP, порт, время, детали).
2. `onEvent` callback вызывает `firewall.RecordHit(ip, type)` — счётчик хитов растёт, при превышении threshold → iptables DROP. Результат (`blocked` / `below_threshold` / `dry_run` / `disabled`) пишется в `event.Details["action_taken"]`.
3. `alerter.Handle(event)` отдаёт событие correlator'у, получает chain + score.
4. Параллельно отправляются:
   - per-event Telegram alert (всегда), с AI-разбором если provider не отключён;
   - chain alert (если score ≥ 50 и не было chain-алерта по этому IP в последние 30 минут).

## Пакеты

```
agent/
├── cmd/goronin/         # main + CLI subcommands (install/start/stop/...)
├── pkg/protocol/        # EventRequest + event/detail константы
└── internal/
    ├── ai/              # 3 провайдера + общий Provider interface
    ├── alerter/         # routing layer: correlator → telegram, с AI
    ├── config/          # YAML + Validate + applyDefaults + Save
    ├── correlator/      # in-memory chains + scoring (порт логики с бывшего бэкенда)
    ├── firewall/        # iptables wrapper + RecordHit + persistent через storage
    ├── storage/         # bbolt: firewall_hits, blocks, meta
    ├── systemd/         # рендер unit-файла + systemctl/journalctl wrappers
    ├── telegram/        # Bot API client + format* функции
    ├── traps/           # SSH/HTTP/FTP/DB listeners + Manager
    ├── watcher/         # fsnotify + canary creation + auto-discovery
    └── wizard/          # интерактивный мастер для install/reconfigure
```

## Хранилище

`/var/lib/goronin/state.db` — bbolt-файл с тремя бакетами:

| Bucket | Key | Value | Назначение |
|---|---|---|---|
| `firewall_hits` | IP | `{ip, count, first_seen, last_seen}` | Счётчики хитов для threshold-бана и эскалации, переживают reboot |
| `blocks` | IP | `{ip, reason, blocked_at, expires_at}` | Активные баны — на старте `RestoreFromStorage` пере-добавляет их в iptables |
| `meta` | string | string | `version`, `last_start` |

Все JSON-encoded, чтобы будущая версия могла читать старые записи и игнорировать новые поля.

## Firewall: жизненный цикл

1. **InitChain** при старте: `iptables -N GORONIN-BLOCK` (no-op если есть), линкуется в INPUT.
2. **RestoreFromStorage**: пере-добавляет неистёкшие баны из bbolt. Истёкшие удаляются из bbolt.
3. **expiryLoop** (раз в 60с): сносит истёкшие баны.
4. **Shutdown**: останавливает expiry. **Не флашит** chain — баны переживают рестарт сервиса.

`ResetChain` (вызывается `goronin reset`) — флашит chain и чистит bbolt-блоки. Hits не трогаются.

## Correlator

In-memory `map[ip]*Chain`. Окно 30 минут. Защита от unlimited-growth: события старше окна вытесняются при каждом `Observe`.

Скоринг (детерминистический, без LLM):
- `+10` за событие
- `+20` за уникальный тип ловушки
- `+30` если есть `file_canary` (signal эксфильтрации)
- `+20` если 3+ событий за 5 минут
- `+40..+50` за известный паттерн (`ssh→canary`, `http→db→canary`, `ftp→canary`)
- clamp [0, 100]

Threshold для chain-алерта: 50. Cooldown между chain-алертами по одному IP: 30 минут.

## AI

Общий интерфейс:

```go
type Provider interface {
    AnalyzeEvent(ctx context.Context, ev EventRequest) (string, error)
    AnalyzeChain(ctx context.Context, ip string, score int, events []EventRequest) (string, error)
}
```

Реализации:
- `anthropicProvider` → `POST https://api.anthropic.com/v1/messages`
- `openaiProvider` → `POST https://api.openai.com/v1/chat/completions`
- `geminiProvider` → `POST https://generativelanguage.googleapis.com/v1beta/models/{model}:generateContent`
- `noopProvider` → возвращает `"", nil` (когда provider не настроен)

Таймаут на каждый AI-вызов — 25 секунд. При ошибке alerter всё равно отправляет Telegram, просто без раздела «AI анализ».

## Telegram

`Send(ctx, html)` — единственный outbound. Format-функции:
- `FormatEventAlert(serverName, event, aiAnalysis)` — per-event
- `FormatChainAlert(serverName, ip, score, events, aiAnalysis)` — chain
- `FormatAgentStartup(serverName, traps)` — при старте daemon

`Verify(ctx)` (используется wizard'ом) — `getMe`, проверяет токен.

## systemd

Unit-файл `/etc/systemd/system/goronin.service`:

```ini
[Service]
Type=simple
ExecStart=/usr/local/bin/goronin daemon
Restart=on-failure
RestartSec=5s
User=root
```

Daemon mode (`goronin daemon`) — то, что реально запускается systemd'ом. `goronin start/stop/restart/status/logs` — обёртки над systemctl/journalctl.

## Что вынесено осознанно

- **Backend**: его нет. Каждая инсталляция автономна.
- **Поллинг бэкенда / pull-конфиг / heartbeat**: не нужны без бэкенда. Конфиг применяется только при `restart`.
- **Сетевая регистрация / токены**: не нужны.
- **Дашборд / web-UI**: алерты идут в Telegram, локально статус через `goronin status`.

## Что осталось в roadmap

См. README → Roadmap. Главное:

1. Real SSH-honeypot на `golang.org/x/crypto/ssh` — сейчас SSH-trap только показывает баннер; с crypto/ssh можно ловить пары `(username, password)`.
2. nftables backend — для дистрибутивов без iptables (Fedora 40+).
3. GitHub Actions release-pipeline — пока бинари нужно собирать локально.
