@echo off
setlocal
cd /d "%~dp0"

if not exist "config\phlo.json" (
  if exist "config\phlo.json.example" (
    copy /y "config\phlo.json.example" "config\phlo.json" >nul
  )
)

if not exist "bin\phlo.exe" (
  call :buildConsole
  if errorlevel 1 exit /b 1
)

call :desktopIcon
if exist "bin\phlo-panel.exe" (
  start "" /D "%~dp0" "%~dp0bin\phlo-panel.exe"
  exit /b 0
)

echo The Phlo window is not available. Opening the dashboard in the browser.
echo Leave the Phlo window open. Closing it stops Phlo.
call :dashboardPort
call :healthy
if not errorlevel 1 (
  start "" "http://127.0.0.1:%PORT%/"
  exit /b 0
)
start "Phlo" /D "%~dp0" "%~dp0bin\phlo.exe" start
set /a TRIES=0
:waitdash
call :healthy
if not errorlevel 1 goto dashopen
set /a TRIES+=1
if %TRIES% GEQ 30 goto dashopen
timeout /t 1 /nobreak >nul
goto waitdash

:dashopen
start "" "http://127.0.0.1:%PORT%/"
exit /b 0

:desktopIcon
where powershell >nul 2>&1
if errorlevel 1 exit /b 0
powershell -NoProfile -WindowStyle Hidden -Command "$root = (Get-Location).Path; $shell = New-Object -ComObject WScript.Shell; $desktop = $shell.SpecialFolders('Desktop'); if (-not $desktop) { exit 0 }; $target = Join-Path $root 'bin\phlo-panel.exe'; if (-not (Test-Path -LiteralPath $target)) { $target = Join-Path $root 'Phlo.vbs' }; $icon = Join-Path $root 'ui\logo.ico'; if (-not (Test-Path -LiteralPath $icon)) { exit 0 }; $link = $shell.CreateShortcut((Join-Path $desktop 'Phlo.lnk')); $link.TargetPath = $target; $link.WorkingDirectory = $root; $link.IconLocation = ($icon + ',0'); $link.Description = 'Phlo'; $link.WindowStyle = 1; $link.Save()"
exit /b 0

:dashboardPort
set "PORT=7700"
where powershell >nul 2>&1
if errorlevel 1 exit /b 0
for /f "usebackq delims=" %%P in (`powershell -NoProfile -Command "try { $j = Get-Content -Raw -LiteralPath 'config\phlo.json' | ConvertFrom-Json; $n = [int]$j.dashboardPort; if ($n -gt 0) { $n } else { 7700 } } catch { 7700 }"`) do set "PORT=%%P"
exit /b 0

:healthy
curl.exe -fsS -o NUL --max-time 1 "http://127.0.0.1:%PORT%/api/health"
if errorlevel 1 exit /b 1
exit /b 0

:buildConsole
call :needGo
if errorlevel 1 exit /b 1
if not exist "bin" mkdir "bin"
echo Building Phlo. The first build can take a minute.
go build -o "bin\phlo.exe" .\cmd\phlo
if errorlevel 1 goto buildFailed
exit /b 0

:needGo
where go >nul 2>&1
if not errorlevel 1 exit /b 0
echo The Phlo program is not in this folder, and Go was not found.
echo Install Go from https://go.dev/dl/ then double-click Phlo.vbs or Phlo.cmd again.
echo.
echo Or build it from this folder:
echo   go build -o bin\phlo.exe .\cmd\phlo
pause
exit /b 1

:buildFailed
echo.
echo The build failed.
pause
exit /b 1
