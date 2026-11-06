@echo off
go mod tidy
if errorlevel 1 exit /b 1
if exist "mdpub.exe" del "mdpub.exe"
go build -o mdpub.exe .
