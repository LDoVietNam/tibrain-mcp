@echo off
REM TiBrain Service Restart
call stop-tibrain.bat
timeout /t 3 /nobreak >nul
call start-tibrain.bat