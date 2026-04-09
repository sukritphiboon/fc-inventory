@echo off
echo ============================================
echo   Building FC Inventory Tool (.exe)
echo ============================================
echo.

pip install pyinstaller requests flask openpyxl

echo.
echo Building executable...
pyinstaller --noconfirm --onedir --name FCInventoryTool ^
    --add-data "templates;templates" ^
    --add-data "static;static" ^
    --hidden-import=mock_data ^
    app.py

echo.
echo ============================================
echo   Build complete!
echo   Output: dist\FCInventoryTool\
echo   Run:    dist\FCInventoryTool\FCInventoryTool.exe
echo ============================================
pause
