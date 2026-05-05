$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot
New-Item -ItemType Directory -Force -Path cli | Out-Null

foreach ($name in @("setup", "connect", "disconnect", "noreconnect")) {
  $env:GOOS = "windows"
  $env:GOARCH = "amd64"
  go build -trimpath -ldflags "-s -w" -o "cli/$name.exe" .
}
