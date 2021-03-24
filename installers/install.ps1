#!/usr/bin/env pwsh
# Copyright 2018 the Deno authors. All rights reserved. MIT license.
# TODO(everyone): Keep this script simple and easily auditable.

[CmdletBinding()]
param (
    [switch]$prerel
)

$ErrorActionPreference = 'Stop'

if ($p) {
  $prerel=TRUE
}

if($prerel) {
  Write-Output "Prerel mode"
}

# if ($v) {
#   $Version = "v${v}"
# }

if ($args.Length -eq 1) {
  $Version = $args.Get(0)
}

Write-Output $Version

$FlyInstall = $env:FLYCTL_INSTALL
$BinDir = if ($FlyInstall) {
  "$FlyInstall\bin"
} else {
  "$Home\.fly\bin"
}

$FlyZip = "$BinDir\flyctl.zip"
$FlyExe = "$BinDir\flyctl.exe"
$RealFlyExe = "$BinDir\fly.exe"
$Target = 'Windows_x86_64'

# GitHub requires TLS 1.2
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

$FlyURI = if (!$Version) {
  $Response = Invoke-WebRequest 'https://github.com/superfly/flyctl/releases' -UseBasicParsing
  $matchstring= "/superfly/flyctl/releases/download/v[0-9]+.[0-9]+.[0-9]+/flyctl_[0-9]+.[0-9]+.[0-9]+_${Target}.tar.gz"
  if($prerel) {
    $matchstring="/superfly/flyctl/releases/download/v[0-9]+.[0-9]+.[0-9]+-beta-[0-9]+/flyctl_[0-9]+.[0-9]+.[0-9]+-beta-[0-9]+_${Target}.tar.gz"
  }
  if ($PSVersionTable.PSEdition -eq 'Core') {
    $Response.Links |
      Where-Object { $_.href -match $matchstring} |
      ForEach-Object { 'https://github.com' + $_.href } |
      Select-Object -First 1
  } else {
    $HTMLFile = New-Object -Com HTMLFile
    if ($HTMLFile.IHTMLDocument2_write) {
      $HTMLFile.IHTMLDocument2_write($Response.Content)
    } else {
      $ResponseBytes = [Text.Encoding]::Unicode.GetBytes($Response.Content)
      $HTMLFile.write($ResponseBytes)
    }
    $HTMLFile.getElementsByTagName('a') |
      Where-Object { $_.href -match "about:"+$matchstring } |
      ForEach-Object { $_.href -replace 'about:', 'https://github.com' } |
      Select-Object -First 1
  }
} else {
  "https://github.com/superfly/flyctl/releases/download/${Version}/flyctl_${Version}_${Target}.tar.gz"
}

Write-Output $FlyUri

if (!(Test-Path $BinDir)) {
  New-Item $BinDir -ItemType Directory | Out-Null
}

Invoke-WebRequest $FlyUri -OutFile $FlyZip -UseBasicParsing

# if (Get-Command Expand-Archive -ErrorAction SilentlyContinue) {
#   Expand-Archive $FlyZip -Destination $BinDir -Force
# } else {
#   if (Test-Path $FlyExe) {
#     Remove-Item $FlyExe
#   }
#   Add-Type -AssemblyName System.IO.Compression.FileSystem
#   [IO.Compression.ZipFile]::ExtractToDirectory($FlyZip, $BinDir)
# }

Push-Location $BinDir
Remove-Item .\flyctl.exe -ErrorAction SilentlyContinue
Remove-Item .\fly.exe -ErrorAction SilentlyContinue
Remove-Item .\wintun.dll -ErrorAction SilentlyContinue
tar -xvzf $FlyZip
Pop-Location

Remove-Item $FlyZip

$User = [EnvironmentVariableTarget]::User
$Path = [Environment]::GetEnvironmentVariable('Path', $User)
if (!(";$Path;".ToLower() -like "*;$BinDir;*".ToLower())) {
  [Environment]::SetEnvironmentVariable('Path', "$Path;$BinDir", $User)
  $Env:Path += ";$BinDir"
}

Start-Process -FilePath "$env:comspec" -ArgumentList "/c", "mklink", $RealFlyExe, $FlyExe

Write-Output "Flyctl was installed successfully to $FlyExe"
Write-Output "Run 'flyctl --help' to get started"