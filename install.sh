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
  C_OK=$'\e[32m'; C_WARN=$'\e[33m'; C_ERR=$'\e[31m'; C_DIM=$'\e[2m'
  C_BOLD=$'\e[1m'; C_CYAN=$'\e[36m'; C_RESET=$'\e[0m'
else
  C_OK=""; C_WARN=""; C_ERR=""; C_DIM=""; C_BOLD=""; C_CYAN=""; C_RESET=""
fi

# ---------- structured output (compact + timings) ----------
START_TS=$SECONDS

# mm:ss since script start
_ts() {
  local elapsed=$((SECONDS - START_TS))
  printf "%02d:%02d" $((elapsed / 60)) $((elapsed % 60))
}

say()  { printf "%s\n" "$*"; }

# step → in-progress action (arrow)
step() { printf "  %s%s%s  %s→%s  %s\n" "$C_DIM" "$(_ts)" "$C_RESET" "$C_CYAN" "$C_RESET" "$*"; }

# ok → completed action (check)
ok()   { printf "  %s%s%s  %s✓%s  %s\n" "$C_DIM" "$(_ts)" "$C_RESET" "$C_OK" "$C_RESET" "$*"; }

# info → neutral note (i)
info() { printf "  %s%s%s  %sⓘ%s  %s\n" "$C_DIM" "$(_ts)" "$C_RESET" "$C_DIM" "$C_RESET" "$*"; }

warn() { printf "  %s%s%s  %s⚠%s  %s\n" "$C_DIM" "$(_ts)" "$C_RESET" "$C_WARN" "$C_RESET" "$*" >&2; }
err()  { printf "  %s%s%s  %s✗%s  %s\n" "$C_DIM" "$(_ts)" "$C_RESET" "$C_ERR" "$C_RESET" "$*" >&2; }
die()  { err "$*"; exit 1; }

# header / footer
header() {
  local version="$1" arch="$2"
  printf "\n%s▶ goronin installer%s · %s%s%s · %slinux-%s%s\n\n" \
    "$C_BOLD" "$C_RESET" "$C_CYAN" "$version" "$C_RESET" "$C_CYAN" "$arch" "$C_RESET"
}

footer() {
  local elapsed=$((SECONDS - START_TS))
  printf "\n  %sinstalled in %ds%s\n\n" "$C_DIM" "$elapsed" "$C_RESET"
  printf "  %snext:%s  goronin health     %s# проверка всех подсистем%s\n" "$C_BOLD" "$C_RESET" "$C_DIM" "$C_RESET"
  printf "         goronin status     %s# статус сервиса%s\n" "$C_DIM" "$C_RESET"
  printf "         goronin logs -f    %s# смотреть логи%s\n" "$C_DIM" "$C_RESET"
  printf "         goronin --help     %s# все команды%s\n\n" "$C_DIM" "$C_RESET"
  printf "  %s─────────────────────────────────────────%s\n" "$C_DIM" "$C_RESET"
  printf "  %sauthor%s    kitay-sudo\n" "$C_DIM" "$C_RESET"
  printf "  %sgithub%s    github.com/kitay-sudo/goronin\n" "$C_DIM" "$C_RESET"
  printf "  %stelegram%s  t.me/kitay9\n\n" "$C_DIM" "$C_RESET"
}

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
  printf "\n%s▶ goronin uninstaller%s\n\n" "$C_BOLD" "$C_RESET"
  if [[ -x "$INSTALL_DIR/$BIN_NAME" ]]; then
    step "удаление GORONIN"
    "$INSTALL_DIR/$BIN_NAME" uninstall
    ok "удалено"
  else
    warn "бинарь $INSTALL_DIR/$BIN_NAME не найден — нечего удалять"
  fi
  printf "\n  %s─────────────────────────────────────────%s\n" "$C_DIM" "$C_RESET"
  printf "  %sauthor%s    kitay-sudo\n" "$C_DIM" "$C_RESET"
  printf "  %sgithub%s    github.com/kitay-sudo/goronin\n" "$C_DIM" "$C_RESET"
  printf "  %stelegram%s  t.me/kitay9\n\n" "$C_DIM" "$C_RESET"
  exit 0
fi

# ---------- detect existing install (decides install vs update later) ----------
EXISTING_BIN=""
EXISTING_CONFIG=""
[[ -x "$INSTALL_DIR/$BIN_NAME" ]] && EXISTING_BIN="yes"
[[ -f "$CONFIG_PATH" ]] && EXISTING_CONFIG="yes"

# ---------- version selection ----------
VERSION="${GORONIN_VERSION:-}"
if [[ -z "$VERSION" ]]; then
  # Print a temporary header before we know the version, then re-emit on confirm.
  printf "\n%s▶ goronin installer%s · %slinux-%s%s\n\n" "$C_BOLD" "$C_RESET" "$C_CYAN" "$ARCH" "$C_RESET"
  step "определение последней версии"
  VERSION="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
    | grep -oE '"tag_name":\s*"[^"]+"' | head -n1 | cut -d'"' -f4 || true)"
  if [[ -z "$VERSION" ]]; then
    warn "не удалось определить последний релиз — fallback на main"
    VERSION="main"
  fi
  ok "версия: $VERSION"
else
  header "$VERSION" "$ARCH"
  ok "версия (закреплена): $VERSION"
fi

if [[ "$MODE" == "reinstall" && -n "$EXISTING_BIN" ]]; then
  warn "режим --reinstall: сношу текущую установку (конфиг и данные будут утеряны)"
  "$INSTALL_DIR/$BIN_NAME" uninstall || true
  EXISTING_BIN=""
  EXISTING_CONFIG=""
fi

# ---------- download ----------
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

BIN_URL="https://github.com/$REPO/releases/download/$VERSION/${BIN_NAME}-linux-${ARCH}"

step "загрузка ${BIN_NAME}-linux-${ARCH}"
if ! curl -fsSL "$BIN_URL" -o "$TMP/$BIN_NAME"; then
  die "не удалось скачать бинарь. проверь, что релиз $VERSION существует на https://github.com/$REPO/releases"
fi
ok "скачано"

chmod +x "$TMP/$BIN_NAME"

# Sanity check
if ! "$TMP/$BIN_NAME" version >/dev/null 2>&1; then
  die "скачанный бинарь не запускается"
fi

# ---------- install binary (always — both fresh install and update) ----------
# `install -m 0755` is atomic on the same filesystem (rename), so even if
# the running daemon has the old inode open, swapping it is safe — the
# kernel keeps the old binary alive until restart.
step "установка бинаря"
install -m 0755 "$TMP/$BIN_NAME" "$INSTALL_DIR/$BIN_NAME"
ok "$INSTALL_DIR/$BIN_NAME ($($INSTALL_DIR/$BIN_NAME version))"

# ---------- branch: update vs fresh install ----------
if [[ -n "$EXISTING_CONFIG" ]]; then
  # Update path: keep the existing config, just bounce the service so the
  # new binary takes over. No wizard — user already configured this box.
  info "конфиг найден ($CONFIG_PATH) — wizard пропущен"
  if systemctl list-unit-files goronin.service >/dev/null 2>&1; then
    step "перезапуск сервиса"
    systemctl restart goronin.service
    ok "goronin.service active"
  else
    warn "unit-файл systemd не найден — запускаю install для регистрации сервиса"
    "$INSTALL_DIR/$BIN_NAME" install
  fi
  footer
  exit 0
fi

# Fresh install: run the wizard. It needs a real terminal — pipe from curl
# is already at EOF, so we reattach stdin to /dev/tty. If there's no tty
# (cron, CI, `ssh -T`), bail out cleanly and tell the user how to finish.
info "свежая установка — запускаю интерактивный мастер"
say
if [[ -e /dev/tty ]]; then
  exec "$INSTALL_DIR/$BIN_NAME" install </dev/tty
else
  warn "нет доступа к /dev/tty — мастер установки не сможет считать ввод"
  say  "  Бинарь уже на месте. Заверши настройку вручную:"
  say  "      sudo $INSTALL_DIR/$BIN_NAME install"
  exit 0
fi
