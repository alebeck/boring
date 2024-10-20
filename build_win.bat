@echo off

for /f "delims=" %%i in ('git describe --tags --exact-match 2^>nul') do set "tag=%%i"
for /f "delims=" %%i in ('git rev-parse --short HEAD 2^>nul') do set "commit=%%i"

if not exist .\bin mkdir .\bin

go build ^
    -ldflags "-X main.version=%tag% -X main.commit=%commit%" ^
    -o .\bin\boring.exe ^
    .\cmd\boring
