@echo off
setlocal

choice /c DR /n /m "Build type - [D]ebug or [R]elease? (D/R): "
if errorlevel 2 goto release
goto debug

:release
set "VER="
set /p "VER=Version number (e.g. v1.0.0): "
if not defined VER (
    echo A version number is required for a release build.
    goto release
)
if /i not "%VER:~0,1%"=="v" set "VER=v%VER%"
set "LDFLAGS=-X main.version=%VER%"
echo Building release %VER%...
goto build

:debug
set "LDFLAGS="
echo Building debug (no version)...

:build
go mod tidy
if errorlevel 1 exit /b 1
if exist "mdpub.exe" del "mdpub.exe"
if defined LDFLAGS (
    go build -ldflags "%LDFLAGS%" -o mdpub.exe .
) else (
    go build -o mdpub.exe .
)
if errorlevel 1 exit /b 1
echo Done: mdpub.exe
endlocal
