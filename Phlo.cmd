@echo off
setlocal
cd /d "%~dp0"

if not exist "bin\phlo.exe" (
  call :buildConsole
  if errorlevel 1 exit /b 1
)
if not exist "bin\phlo-panel.exe" (
  call :buildWindow
  if errorlevel 1 exit /b 1
)
if not exist "config\phlo.json" (
  if exist "config\phlo.json.example" (
    copy /y "config\phlo.json.example" "config\phlo.json" >nul
  )
)

start "" /D "%~dp0" "%~dp0bin\phlo-panel.exe"
exit /b 0

:buildConsole
call :needGo
if errorlevel 1 exit /b 1
if not exist "bin" mkdir "bin"
echo Building Phlo. The first build can take a minute.
go build -o "bin\phlo.exe" .\cmd\phlo
if errorlevel 1 goto buildFailed
exit /b 0

:buildWindow
call :needGo
if errorlevel 1 exit /b 1
if not exist "bin" mkdir "bin"
echo Building the Phlo window. The first build can take a minute.
go build -ldflags "-H windowsgui" -o "bin\phlo-panel.exe" .\cmd\phlo-panel
if errorlevel 1 goto buildFailed
exit /b 0

:needGo
where go >nul 2>&1
if not errorlevel 1 exit /b 0
echo The Phlo program is not in this folder, and Go was not found.
echo Install Go from https://go.dev/dl/ and double-click Phlo.cmd again.
echo.
echo Or build it from this folder:
echo   go build -o bin\phlo.exe .\cmd\phlo
echo   go build -ldflags "-H windowsgui" -o bin\phlo-panel.exe .\cmd\phlo-panel
pause
exit /b 1

:buildFailed
echo.
echo The build failed.
pause
exit /b 1
