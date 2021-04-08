#!/usr/bin/env pwsh
# Copyright 2018 the Deno authors. All rights reserved. MIT license.
# TODO(everyone): Keep this script simple and easily auditable.

$ErrorActionPreference = 'Stop'

$Version = if ($v) {
  $v
} elseif ($args.Length -eq 1) {
  $args.Get(0)
} else {
  "latest"
}

$FlyInstall = $env:FLYCTL_INSTALL
$BinDir = if ($FlyInstall) {
  "$FlyInstall\bin"
} else {
  "$Home\.fly\bin"
}

$FlyZip = "$BinDir\flyctl.zip"
$FlyctlExe = "$BinDir\flyctl.exe"
$WintunDll = "$BinDir\wintun.dll"
$FlyExe = "$BinDir\fly.exe"

# Fly & GitHub require TLS 1.2
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

try {
  $Response = Invoke-WebRequest "https://api.fly.io/app/flyctl_releases/windows/x86_64/$Version" -UseBasicParsing
  $FlyUri = $Response.Content
}
catch {
  $StatusCode = $_.Exception.Response.StatusCode.value__
  if ($StatusCode -eq 404) {
    Write-Error "Unable to find a flyctl release on GitHub for version:$Version - see github.com/superfly/flyctl/releases for all versions"
  } else {
    $Request = $_.Exception
    Write-Error "Error while fetching releases: $Request"
  }
  Exit 1
}

if (!(Test-Path $BinDir)) {
  New-Item $BinDir -ItemType Directory | Out-Null
}

Invoke-WebRequest $FlyUri -OutFile $FlyZip -UseBasicParsing

if (Get-Command Expand-Archive -ErrorAction SilentlyContinue) {
  Expand-Archive $FlyZip -Destination $BinDir -Force
} else {
  Remove-Item $FlyctlExe -ErrorAction SilentlyContinue
  Remove-Item $FlyExe -ErrorAction SilentlyContinue
  Remove-Item $WintunDll -ErrorAction SilentlyContinue
  Add-Type -AssemblyName System.IO.Compression.FileSystem
  [IO.Compression.ZipFile]::ExtractToDirectory($FlyZip, $BinDir)
}

Remove-Item $FlyZip

$User = [EnvironmentVariableTarget]::User
$Path = [Environment]::GetEnvironmentVariable('Path', $User)
if (!(";$Path;".ToLower() -like "*;$BinDir;*".ToLower())) {
  [Environment]::SetEnvironmentVariable('Path', "$Path;$BinDir", $User)
  $Env:Path += ";$BinDir"
}

Start-Process -FilePath "$env:comspec" -ArgumentList "/c", "mklink", $FlyExe, $FlyctlExe

Write-Output "flyctl was installed successfully to $FlyctlExe"
Write-Output "Run 'flyctl --help' to get started"