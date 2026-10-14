@echo off
setlocal
cd /d "%~dp0"

if not exist "bin\phlo-panel.exe" (
  echo Build the Phlo window first:
  echo   go build -ldflags "-H windowsgui" -o bin\phlo-panel.exe .\cmd\phlo-panel
  pause
  exit /b 1
)

start "" /D "%~dp0" "%~dp0bin\phlo-panel.exe"
exit /b 0
