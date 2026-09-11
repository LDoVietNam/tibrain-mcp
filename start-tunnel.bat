@echo off
title TiBrain Tunnel Manager

echo ========================================
echo TiBrain Tunnel Manager
echo ========================================

REM Kill any existing tunnel processes
echo [1/3] Cleaning up existing processes...
taskkill /IM cloudflared.exe /F 2>nul
taskkill /IM tibrain.exe /F 2>nul

REM Wait for ports to be released
echo [2/3] Waiting for ports to be released...
timeout /t 3 /nobreak >nul

REM Start TiBrain server
echo [3/3] Starting TiBrain server...
cd /d %~dp0
start "" tibrain.exe

REM Wait for server to start
timeout /t 2 /nobreak >nul

REM Start Cloudflare tunnel
echo Starting Cloudflare tunnel...
cloudflared tunnel run mcp-trepremium

pause