# Установка GORONIN

## Быстрый путь (рекомендуемый)

```bash
curl -sSL https://raw.githubusercontent.com/kitay-sudo/goronin/main/install.sh | sudo bash
```

Скрипт сделает всё сам: определит архитектуру, скачает бинарь, поставит в `/usr/local/bin/goronin`, запустит интерактивный wizard, поднимет systemd-сервис.

## Что нужно подготовить заранее

### Telegram бот

1. В Telegram напиши `@BotFather`, команду `/newbot`, придумай имя — получишь токен вида `1234567890:AAEhBOOO...`.
2. Узнай свой chat_id: напиши `@userinfobot`, он ответит твоим ID. Если хочешь алерты в группу — добавь бота в группу и используй её chat_id (для групп он отрицательный, например `-1001234567890`).

### AI-провайдер (опционально)

Можно пропустить — алерты будут приходить без AI-разбора. Если хочешь подключить, выбери один:

- **Anthropic Claude**: ключ на https://console.anthropic.com/settings/keys (формат `sk-ant-...`)
- **OpenAI**: ключ на https://platform.openai.com/api-keys (формат `sk-...`)
- **Google Gemini**: ключ на https://aistudio.google.com/apikey

## Системные требования

- Linux с systemd (Ubuntu 18+, Debian 9+, CentOS 7+, Rocky, Alma, Arch, Alpine с openrc-systemd-shim)
- iptables (для авто-бана; без него агент работает, но в режиме «только алерты»)
- curl (для install.sh)
- root (для bind на порты, iptables и записи systemd-юнита)

Архитектуры: `amd64` (x86_64), `arm64` (aarch64).

## Установка вручную (без curl)

Если не доверяешь `curl | bash`:

1. Открой https://github.com/kitay-sudo/goronin/releases/latest и скачай `goronin-linux-amd64` (или `arm64`).
2. ```bash
   sudo install -m 0755 goronin-linux-amd64 /usr/local/bin/goronin
   sudo /usr/local/bin/goronin install
   ```

## Сборка из исходников

```bash
git clone https://github.com/kitay-sudo/goronin.git
cd goronin/agent
go build -ldflags "-X main.version=$(git describe --tags --always)" -o goronin ./cmd/goronin
sudo install -m 0755 goronin /usr/local/bin/
sudo /usr/local/bin/goronin install
```

Минимальная версия Go — 1.25.

## После установки

```bash
goronin status              # проверить, что сервис активен
goronin logs -f             # смотреть события в реальном времени
```

В Telegram должно прийти стартовое сообщение со списком ловушек и портами.

## Удаление

```bash
sudo systemctl stop goronin
sudo systemctl disable goronin
sudo rm /etc/systemd/system/goronin.service
sudo rm /usr/local/bin/goronin
sudo rm -rf /etc/goronin /var/lib/goronin
sudo iptables -D INPUT -j GORONIN-BLOCK 2>/dev/null
sudo iptables -F GORONIN-BLOCK 2>/dev/null
sudo iptables -X GORONIN-BLOCK 2>/dev/null
sudo systemctl daemon-reload
```
