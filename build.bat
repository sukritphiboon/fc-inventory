@echo off
REM ============================================
REM   FC Inventory Tool - Production Build
REM ============================================

echo Installing dependencies...
pip install -r requirements.txt
pip install pyinstaller

echo.
echo Cleaning previous build...
if exist build rmdir /s /q build
if exist dist rmdir /s /q dist
if exist FCInventoryTool.spec del /f /q FCInventoryTool.spec

echo.
echo Building FCInventoryTool.exe ...
pyinstaller --noconfirm --onedir --name FCInventoryTool ^
    --add-data "templates;templates" ^
    --add-data "static;static" ^
    --hidden-import waitress ^
    --collect-submodules waitress ^
    app.py

echo.
echo ============================================
echo   Build complete!
echo.
echo   Output: dist\FCInventoryTool\
echo   Run:    dist\FCInventoryTool\FCInventoryTool.exe
echo ============================================
pause
