#!/usr/bin/env bash
# GORONIN — one-command installer / updater / uninstaller.
#
# Usage:
#   # Первая установка ИЛИ обновление до последней версии (одна команда):
#   curl -sSL https://raw.githubusercontent.com/kitay-sudo/goronin/main/install.sh | sudo bash
#
#   # Принудительная переустановка с нуля (снесёт конфиг и данные!):
#   curl -sSL .../install.sh | sudo bash -s -- --reinstall
#
#   # Только удаление:
#   curl -sSL .../install.sh | sudo bash -s -- --uninstall
#
# Поведение зависит от того, что уже есть на сервере:
#   - бинаря нет          → скачать + запустить wizard (fresh install)
#   - бинарь и конфиг     → скачать новую версию, заменить бинарь, рестарт сервиса
#                           (wizard НЕ запускается, настройки сохраняются)
#   - --reinstall         → uninstall + fresh install (потеря конфига!)
#   - --uninstall         → только зачистить
#
# Закрепить версию: GORONIN_VERSION=v0.3.1 перед командой.

set -euo pipefail

REPO="kitay-sudo/goronin"
INSTALL_DIR="/usr/local/bin"
BIN_NAME="goronin"
CONFIG_PATH="/etc/goronin/config.yml"

MODE="auto" # auto | reinstall | uninstall
for arg in "$@"; do
  case "$arg" in
    --reinstall) MODE="reinstall" ;;
    --uninstall) MODE="uninstall" ;;
    -h|--help)
      sed -n '2,22p' "$0" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    *) echo "Неизвестный флаг: $arg" >&2; exit 1 ;;
  esac
done

# ---------- coloring ----------
if [[ -t 1 ]]; then
  C_OK=$'\e[32m'; C_WARN=$'\e[33m'; C_ERR=$'\e[31m'; C_DIM=$'\e[2m'; C_RESET=$'\e[0m'
else
  C_OK=""; C_WARN=""; C_ERR=""; C_DIM=""; C_RESET=""
fi

say()  { printf "%s\n" "$*"; }
ok()   { printf "%s✓%s %s\n" "$C_OK" "$C_RESET" "$*"; }
warn() { printf "%s⚠%s %s\n" "$C_WARN" "$C_RESET" "$*" >&2; }
err()  { printf "%s✗%s %s\n" "$C_ERR" "$C_RESET" "$*" >&2; }
die()  { err "$*"; exit 1; }

# ---------- preflight ----------
[[ "$EUID" -eq 0 ]] || die "Запусти от root: curl ... | sudo bash"

OS="$(uname -s)"
[[ "$OS" == "Linux" ]] || die "Поддерживается только Linux (сейчас: $OS)"

case "$(uname -m)" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) die "Неподдерживаемая архитектура: $(uname -m)" ;;
esac

command -v curl >/dev/null 2>&1 || die "curl не найден (sudo apt install curl / sudo yum install curl)"

if ! command -v iptables >/dev/null 2>&1; then
  warn "iptables не найден — авто-бан будет недоступен. Установка продолжается, ловушки и алерты работать будут."
fi

if ! command -v systemctl >/dev/null 2>&1; then
  die "systemd не найден — поддерживаются только дистрибутивы с systemd (Ubuntu/Debian/CentOS/Arch/...)"
fi

# ---------- uninstall path (no download needed) ----------
if [[ "$MODE" == "uninstall" ]]; then
  if [[ -x "$INSTALL_DIR/$BIN_NAME" ]]; then
    say "${C_DIM}Удаляю GORONIN…${C_RESET}"
    "$INSTALL_DIR/$BIN_NAME" uninstall
  else
    warn "Бинарь $INSTALL_DIR/$BIN_NAME не найден — нечего удалять."
  fi
  exit 0
fi

# ---------- detect existing install (decides install vs update later) ----------
EXISTING_BIN=""
EXISTING_CONFIG=""
[[ -x "$INSTALL_DIR/$BIN_NAME" ]] && EXISTING_BIN="yes"
[[ -f "$CONFIG_PATH" ]] && EXISTING_CONFIG="yes"

if [[ "$MODE" == "reinstall" && -n "$EXISTING_BIN" ]]; then
  warn "Режим --reinstall: сношу текущую установку (конфиг и данные будут утеряны)."
  "$INSTALL_DIR/$BIN_NAME" uninstall || true
  EXISTING_BIN=""
  EXISTING_CONFIG=""
fi

# ---------- version selection ----------
VERSION="${GORONIN_VERSION:-}"
if [[ -z "$VERSION" ]]; then
  say "${C_DIM}Определяю последнюю версию…${C_RESET}"
  VERSION="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
    | grep -oE '"tag_name":\s*"[^"]+"' | head -n1 | cut -d'"' -f4 || true)"
  if [[ -z "$VERSION" ]]; then
    warn "Не удалось определить последний релиз — пробую main-ветку через git clone"
    VERSION="main"
  fi
fi
ok "Версия: $VERSION"
ok "Архитектура: linux-$ARCH"

# ---------- download ----------
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

BIN_URL="https://github.com/$REPO/releases/download/$VERSION/${BIN_NAME}-linux-${ARCH}"

say "${C_DIM}Скачиваю $BIN_URL${C_RESET}"
if ! curl -fsSL "$BIN_URL" -o "$TMP/$BIN_NAME"; then
  die "Не удалось скачать бинарь. Проверь, что релиз $VERSION существует на https://github.com/$REPO/releases"
fi

chmod +x "$TMP/$BIN_NAME"

# Sanity check
if ! "$TMP/$BIN_NAME" version >/dev/null 2>&1; then
  die "Скачанный бинарь не запускается"
fi

# ---------- install binary (always — both fresh install and update) ----------
# `install -m 0755` is atomic on the same filesystem (rename), so even if
# the running daemon has the old inode open, swapping it is safe — the
# kernel keeps the old binary alive until restart.
install -m 0755 "$TMP/$BIN_NAME" "$INSTALL_DIR/$BIN_NAME"
ok "Бинарь: $INSTALL_DIR/$BIN_NAME ($($INSTALL_DIR/$BIN_NAME version))"

# ---------- branch: update vs fresh install ----------
if [[ -n "$EXISTING_CONFIG" ]]; then
  # Update path: keep the existing config, just bounce the service so the
  # new binary takes over. No wizard — user already configured this box.
  say
  say "${C_DIM}Обнаружен существующий конфиг ($CONFIG_PATH) — обновляю без перезапроса настроек.${C_RESET}"
  if systemctl list-unit-files goronin.service >/dev/null 2>&1; then
    systemctl restart goronin.service
    ok "Сервис перезапущен с новой версией"
  else
    warn "Unit-файл systemd не найден — запускаю install для регистрации сервиса."
    "$INSTALL_DIR/$BIN_NAME" install
  fi
  say
  say "Управление: ${C_OK}goronin status | logs -f | restart${C_RESET}"
  say "Изменить настройки: ${C_OK}sudo goronin reconfigure${C_RESET}"
  say "Удалить полностью:  ${C_OK}sudo goronin uninstall${C_RESET}  (или ${C_OK}--uninstall${C_RESET} в этом скрипте)"
  exit 0
fi

# Fresh install: run the wizard. It needs a real terminal — pipe from curl
# is already at EOF, so we reattach stdin to /dev/tty. If there's no tty
# (cron, CI, `ssh -T`), bail out cleanly and tell the user how to finish.
say
say "${C_DIM}Свежая установка — запускаю интерактивный мастер…${C_RESET}"
say
if [[ -e /dev/tty ]]; then
  exec "$INSTALL_DIR/$BIN_NAME" install </dev/tty
else
  warn "Нет доступа к /dev/tty — мастер установки не сможет считать ввод."
  say  "Бинарь уже на месте. Заверши настройку вручную:"
  say  "    sudo $INSTALL_DIR/$BIN_NAME install"
  exit 0
fi
