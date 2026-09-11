@echo off
REM Safe build script for TiBrain - uses Go from Z:\09_TOOLS\go
REM Usage: Double-click or run from command prompt

set GOTOOLCHAIN=auto
set GO_BIN=Z:\09_TOOLS\go\bin\go.exe
set PROJECT_DIR=Z:\01_PROJECTS\apps\products\tibrain
set RELEASES_DIR=%PROJECT_DIR%\bin\releases
set BINARY_NAME=tibrain.exe

echo === TiBrain Safe Build ===
echo Go binary: %GO_BIN%

cd /d %PROJECT_DIR%

if not exist %RELEASES_DIR% mkdir %RELEASES_DIR%

echo Building to: %RELEASES_DIR%\%BINARY_NAME%
%GO_BIN% build -o %RELEASES_DIR%\%BINARY_NAME% .

if exist %RELEASES_DIR%\%BINARY_NAME% (
    echo Build successful: %RELEASES_DIR%\%BINARY_NAME%
) else (
    echo Build failed!
    exit 1
)

echo === Build Complete ===
pause
