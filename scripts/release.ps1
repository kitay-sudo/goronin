# release.ps1 — bump SemVer, create git tag, push.
#
# Использование (из любой папки репо):
#   .\scripts\release.ps1 patch    # v0.2.1 -> v0.2.2  (баг-фиксы)
#   .\scripts\release.ps1 minor    # v0.2.1 -> v0.3.0  (новые фичи, без ломки совместимости)
#   .\scripts\release.ps1 major    # v0.2.1 -> v1.0.0  (ломающие изменения)
#   .\scripts\release.ps1 v0.4.0   # явная версия
#
# Что делает:
#   1. Проверяет что ты на main, working tree чистый, локальный = origin.
#   2. Берёт последний vX.Y.Z тег и считает следующий.
#   3. Показывает коммиты которые войдут в релиз и просит подтверждение.
#   4. Создаёт annotated тег и пушит. Workflow release.yml собирает бинари.

param(
    [Parameter(Mandatory = $true, Position = 0)]
    [string]$Bump
)

$ErrorActionPreference = 'Stop'

# Перейти в корень репо (на уровень выше папки scripts/)
$repoRoot = Split-Path -Parent $PSScriptRoot
Set-Location $repoRoot

function Write-Ok    { param($msg) Write-Host "OK  $msg" -ForegroundColor Green }
function Write-Warn  { param($msg) Write-Host "!   $msg" -ForegroundColor Yellow }
function Write-Err   { param($msg) Write-Host "X   $msg" -ForegroundColor Red }
function Die         { param($msg) Write-Err $msg; exit 1 }

# ---------- preflight ----------
$branch = (git rev-parse --abbrev-ref HEAD).Trim()
if ($branch -ne 'main') {
    Die "Релиз делается с main (сейчас: $branch). Сначала: git checkout main"
}

$dirty = git status --porcelain
if ($dirty) {
    Die "Working tree не чистый. Закоммить или спрячь изменения перед релизом."
}

Write-Host "git fetch --tags..." -ForegroundColor DarkGray
git fetch --tags --quiet

$local  = (git rev-parse '@').Trim()
$remote = (git rev-parse '@{u}' 2>$null)
if ($LASTEXITCODE -ne 0) { $remote = $local }
$remote = $remote.Trim()

if ($local -ne $remote) {
    Die "Локальный main не совпадает с origin/main. Сначала: git pull --rebase или git push"
}

# Уже отмечен релизным тегом — нечего делать.
$existing = git tag --points-at HEAD | Where-Object { $_ -match '^v\d+\.\d+\.\d+$' } | Select-Object -First 1
if ($existing) {
    Write-Warn "HEAD уже отмечен тегом $existing — релиз уже был. Выходим."
    exit 0
}

# ---------- compute next version ----------
$lastTag = git tag --list 'v[0-9]*.[0-9]*.[0-9]*' --sort=-v:refname | Select-Object -First 1
if (-not $lastTag) { $lastTag = 'v0.0.0' }
Write-Host "Последний тег: $lastTag" -ForegroundColor DarkGray

if ($Bump -match '^v\d+\.\d+\.\d+$') {
    $next = $Bump
}
elseif ($Bump -in @('patch', 'minor', 'major')) {
    $version = $lastTag.TrimStart('v')
    $parts = $version.Split('.')
    $maj = [int]$parts[0]
    $min = [int]$parts[1]
    $pat = [int]$parts[2]

    switch ($Bump) {
        'major' { $maj++; $min = 0; $pat = 0 }
        'minor' { $min++; $pat = 0 }
        'patch' { $pat++ }
    }
    $next = "v$maj.$min.$pat"
}
else {
    Die "Не понимаю аргумент: $Bump. Ожидается patch / minor / major или vX.Y.Z"
}

# Тег уже существует?
git rev-parse $next 2>$null | Out-Null
if ($LASTEXITCODE -eq 0) {
    Die "Тег $next уже существует. Удали его или выбери другую версию."
}

# ---------- show commits and ask confirmation ----------
Write-Host ""
Write-Host "Релиз: " -NoNewline; Write-Host $next -ForegroundColor Green -NoNewline; Write-Host "  (предыдущий: $lastTag)"
Write-Host ""
Write-Host "Коммиты с прошлого тега:"
git log --oneline --no-decorate "$lastTag..HEAD" | ForEach-Object { Write-Host "  $_" }
Write-Host ""

$answer = Read-Host "Создать тег $next и запушить? [y/N]"
if ($answer -notmatch '^(y|yes)$') {
    Write-Warn "Отменено."
    exit 0
}

# ---------- tag and push ----------
git tag -a $next -m $next
if ($LASTEXITCODE -ne 0) { Die "git tag не сработал" }

git push origin $next
if ($LASTEXITCODE -ne 0) { Die "git push не сработал" }

Write-Ok "Тег $next запушен."
Write-Host ""
Write-Host "Workflow release.yml уже стартовал. Следить за прогрессом:"
Write-Host "  https://github.com/kitay-sudo/goronin/actions" -ForegroundColor DarkGray
Write-Host ""
Write-Host "Через 2-3 минуты бинари появятся в:"
Write-Host "  https://github.com/kitay-sudo/goronin/releases/tag/$next" -ForegroundColor DarkGray
