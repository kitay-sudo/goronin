@echo off
setlocal EnableDelayedExpansion

REM release.bat - bump SemVer, create git tag, push.
REM
REM Usage (from repo root):
REM   scripts\release.bat patch    v0.2.0 -> v0.2.1  (bug fixes)
REM   scripts\release.bat minor    v0.2.0 -> v0.3.0  (new features)
REM   scripts\release.bat major    v0.2.0 -> v1.0.0  (breaking changes)
REM
REM Calls git, no PowerShell, no bash, no WSL.

if "%~1"=="" (
    echo Usage: %~nx0 ^<patch^|minor^|major^|vX.Y.Z^>
    exit /b 1
)

cd /d "%~dp0\.."

REM ---------- preflight ----------
for /f "delims=" %%b in ('git rev-parse --abbrev-ref HEAD') do set BRANCH=%%b
if not "!BRANCH!"=="main" (
    echo X   Release must be made from main, current: !BRANCH!
    echo     Run: git checkout main
    exit /b 1
)

git diff --quiet HEAD
if errorlevel 1 (
    echo X   Working tree is not clean. Commit or stash first.
    exit /b 1
)
git diff --cached --quiet
if errorlevel 1 (
    echo X   Index has staged changes. Commit or reset first.
    exit /b 1
)

echo git fetch --tags...
git fetch --tags --quiet

for /f "delims=" %%a in ('git rev-parse "@"') do set LOCAL=%%a
for /f "delims=" %%a in ('git rev-parse "@{u}" 2^>nul') do set REMOTE=%%a
if "!REMOTE!"=="" set REMOTE=!LOCAL!

if not "!LOCAL!"=="!REMOTE!" (
    echo X   Local main does not match origin/main.
    echo     Run: git pull --rebase OR git push
    exit /b 1
)

REM ---------- last tag ----------
set LAST_TAG=v0.0.0
for /f "delims=" %%t in ('git tag --list "v*.*.*" --sort=-v:refname') do (
    set LAST_TAG=%%t
    goto :got_tag
)
:got_tag
echo Last tag: !LAST_TAG!

REM ---------- compute next ----------
set BUMP=%~1

REM Explicit version like v0.4.0 - take it as-is.
echo !BUMP! | findstr /r /c:"^v[0-9][0-9]*\.[0-9][0-9]*\.[0-9][0-9]*$" >nul
if not errorlevel 1 (
    set NEXT=!BUMP!
    goto :have_version
)

REM Otherwise must be patch / minor / major.
if /i not "!BUMP!"=="patch" if /i not "!BUMP!"=="minor" if /i not "!BUMP!"=="major" (
    echo X   Unknown argument: !BUMP!
    echo     Expected: patch ^| minor ^| major ^| vX.Y.Z
    exit /b 1
)

REM Parse vMAJOR.MINOR.PATCH from LAST_TAG.
set VER=!LAST_TAG:v=!
for /f "tokens=1,2,3 delims=." %%a in ("!VER!") do (
    set MAJ=%%a
    set MIN=%%b
    set PAT=%%c
)

if /i "!BUMP!"=="major" (
    set /a MAJ=MAJ+1
    set MIN=0
    set PAT=0
)
if /i "!BUMP!"=="minor" (
    set /a MIN=MIN+1
    set PAT=0
)
if /i "!BUMP!"=="patch" (
    set /a PAT=PAT+1
)

set NEXT=v!MAJ!.!MIN!.!PAT!

:have_version

REM Tag already exists?
git rev-parse "!NEXT!" >nul 2>&1
if not errorlevel 1 (
    echo X   Tag !NEXT! already exists.
    exit /b 1
)

REM ---------- show commits and ask confirmation ----------
echo.
echo Release: !NEXT!  (previous: !LAST_TAG!)
echo.
echo Commits since last tag:
git log --oneline --no-decorate "!LAST_TAG!..HEAD"
echo.

set /p ANSWER=Create tag !NEXT! and push? [y/N]:
if /i not "!ANSWER!"=="y" if /i not "!ANSWER!"=="yes" (
    echo !   Cancelled.
    exit /b 0
)

REM ---------- tag and push ----------
git tag -a "!NEXT!" -m "!NEXT!"
if errorlevel 1 (
    echo X   git tag failed
    exit /b 1
)

git push origin "!NEXT!"
if errorlevel 1 (
    echo X   git push failed
    exit /b 1
)

echo.
echo OK  Tag !NEXT! pushed.
echo.
echo Workflow release.yml has started. Track at:
echo   https://github.com/kitay-sudo/goronin/actions
echo.
echo In 2-3 minutes binaries appear at:
echo   https://github.com/kitay-sudo/goronin/releases/tag/!NEXT!

endlocal
