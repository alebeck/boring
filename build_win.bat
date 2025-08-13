@echo off

for /f "delims=" %%i in ('git describe --tags --exact-match 2^>nul') do set "tag=%%i"
for /f "delims=" %%i in ('git rev-parse --short HEAD 2^>nul') do set "commit=%%i"

if not exist .\dist mkdir .\dist

go build ^
    -ldflags "-X main.version=%tag% -X main.commit=%commit%" ^
    -o .\dist\boring.exe ^
    .\cmd\boring
