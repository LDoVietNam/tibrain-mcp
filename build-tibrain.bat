@echo off
cd /d "%~dp0"
echo [TiBrain] Building binary moi nhat...
set GOOS=windows
set GOARCH=amd64
go build -o Z:\03_DATA\bin\tibrain.exe .
if errorlevel 1 (
    echo [TiBrain] LOI: Build that bai!
    pause
    exit /b 1
)
echo [TiBrain] Build thanh cong: Z:\03_DATA\bin\tibrain.exe
pause