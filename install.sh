#!/usr/bin/env bash
# GORONIN — one-command installer.
#
# Usage:
#   curl -sSL https://raw.githubusercontent.com/kitay-sudo/goronin/main/install.sh | sudo bash
#
# What it does:
#   1. Detects Linux distro and CPU arch.
#   2. Checks for iptables (required for auto-ban; warns if absent).
#   3. Downloads the matching binary from the latest GitHub Release.
#   4. Installs to /usr/local/bin/goronin.
#   5. Runs `goronin install` — interactive wizard for Telegram, AI, traps.
#      The wizard writes /etc/goronin/config.yml, registers the systemd unit,
#      and starts the service.
#
# To pin a version, set GORONIN_VERSION=v0.3.1 before piping into bash.

set -euo pipefail

REPO="kitay-sudo/goronin"
INSTALL_DIR="/usr/local/bin"
BIN_NAME="goronin"

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

# ---------- install ----------
install -m 0755 "$TMP/$BIN_NAME" "$INSTALL_DIR/$BIN_NAME"
ok "Установлен: $INSTALL_DIR/$BIN_NAME ($($INSTALL_DIR/$BIN_NAME version))"

say
say "${C_DIM}Запускаю интерактивную установку…${C_RESET}"
say
exec "$INSTALL_DIR/$BIN_NAME" install
