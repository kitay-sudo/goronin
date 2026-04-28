#!/usr/bin/env bash
# release.sh — bump SemVer, create git tag, push.
#
# Использование:
#   ./scripts/release.sh patch    # v0.2.1 → v0.2.2  (баг-фиксы)
#   ./scripts/release.sh minor    # v0.2.1 → v0.3.0  (новые фичи, без ломки совместимости)
#   ./scripts/release.sh major    # v0.2.1 → v1.0.0  (ломающие изменения)
#   ./scripts/release.sh v0.4.0   # явная версия (если автобамп не подходит)
#
# Что делает:
#   1. Проверяет что ты на main и working tree чистый.
#   2. git fetch — чтобы видеть свежие теги с remote.
#   3. Берёт последний `vX.Y.Z` тег, считает следующий по типу bump.
#   4. Создаёт annotated тег и пушит его → workflow `release.yml` собирает
#      бинари linux/amd64 + arm64 и публикует GitHub Release.
#
# Скрипт идемпотентен в том смысле, что не создаёт тег если HEAD уже
# отмечен релизным тегом — просто скажет об этом и выйдет.

set -euo pipefail

cd "$(dirname "$0")/.."

C_OK=$'\e[32m'; C_WARN=$'\e[33m'; C_ERR=$'\e[31m'; C_DIM=$'\e[2m'; C_RESET=$'\e[0m'
ok()   { printf "%s✓%s %s\n" "$C_OK" "$C_RESET" "$*"; }
warn() { printf "%s⚠%s %s\n" "$C_WARN" "$C_RESET" "$*" >&2; }
die()  { printf "%s✗%s %s\n" "$C_ERR" "$C_RESET" "$*" >&2; exit 1; }

# ---------- проверки ----------
[[ $# -eq 1 ]] || die "Использование: $0 {patch|minor|major|vX.Y.Z}"

BRANCH="$(git rev-parse --abbrev-ref HEAD)"
[[ "$BRANCH" == "main" ]] || die "Релиз делается с main (сейчас: $BRANCH). Сначала git checkout main."

if [[ -n "$(git status --porcelain)" ]]; then
  die "Working tree не чистый. Закоммить или спрячь изменения перед релизом."
fi

printf "%s\n" "${C_DIM}git fetch --tags…${C_RESET}"
git fetch --tags --quiet

# Если удалённый main впереди — мы выпустим тег от старого коммита, не желательно.
LOCAL="$(git rev-parse @)"
REMOTE="$(git rev-parse @{u} 2>/dev/null || echo "$LOCAL")"
if [[ "$LOCAL" != "$REMOTE" ]]; then
  die "Локальный main не совпадает с origin/main. Сначала git pull --rebase / git push."
fi

# Уже отмечен релизным тегом — нечего релизить.
if EXISTING="$(git tag --points-at HEAD | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | head -n1)"; then
  if [[ -n "$EXISTING" ]]; then
    warn "HEAD уже отмечен тегом $EXISTING — релиз уже был. Выходим."
    exit 0
  fi
fi

# ---------- вычисление следующей версии ----------
LAST_TAG="$(git tag --list 'v[0-9]*.[0-9]*.[0-9]*' --sort=-v:refname | head -n1 || true)"
[[ -n "$LAST_TAG" ]] || LAST_TAG="v0.0.0"
echo "Последний тег: ${C_DIM}$LAST_TAG${C_RESET}"

case "$1" in
  patch|minor|major)
    VERSION="${LAST_TAG#v}"
    IFS='.' read -r MAJOR MINOR PATCH <<< "$VERSION"
    case "$1" in
      major) MAJOR=$((MAJOR + 1)); MINOR=0; PATCH=0 ;;
      minor) MINOR=$((MINOR + 1)); PATCH=0 ;;
      patch) PATCH=$((PATCH + 1)) ;;
    esac
    NEXT="v${MAJOR}.${MINOR}.${PATCH}"
    ;;
  v[0-9]*.[0-9]*.[0-9]*)
    NEXT="$1"
    ;;
  *)
    die "Не понимаю аргумент: $1. Ожидается patch / minor / major или vX.Y.Z."
    ;;
esac

# Проверим что такого тега ещё нет (на случай явной vX.Y.Z которая уже занята).
if git rev-parse "$NEXT" >/dev/null 2>&1; then
  die "Тег $NEXT уже существует. Удали его (git tag -d $NEXT && git push origin :$NEXT) или выбери другую версию."
fi

# ---------- покажем коммиты которые войдут в релиз и попросим подтверждения ----------
echo
echo "Релиз: ${C_OK}$NEXT${C_RESET}  (предыдущий: $LAST_TAG)"
echo
echo "Коммиты с прошлого тега:"
git log --oneline --no-decorate "${LAST_TAG}..HEAD" | sed 's/^/  /'
echo

read -r -p "Создать тег $NEXT и запушить? [y/N] " ANSWER
case "$ANSWER" in
  y|Y|yes|YES) ;;
  *) warn "Отменено."; exit 0 ;;
esac

# ---------- тегаем и пушим ----------
git tag -a "$NEXT" -m "$NEXT"
git push origin "$NEXT"

ok "Тег $NEXT запушен."
echo
echo "Workflow release.yml уже стартовал. Следить за прогрессом:"
echo "  ${C_DIM}https://github.com/kitay-sudo/goronin/actions${C_RESET}"
echo
echo "Через 2-3 минуты бинари появятся в:"
echo "  ${C_DIM}https://github.com/kitay-sudo/goronin/releases/tag/$NEXT${C_RESET}"
