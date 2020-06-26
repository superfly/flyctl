#!/usr/bin/env pwsh
# Copyright 2018 the Deno authors. All rights reserved. MIT license.
# TODO(everyone): Keep this script simple and easily auditable.

$ErrorActionPreference = 'Stop'

if ($v) {
  $Version = "v${v}"
}
if ($args.Length -eq 1) {
  $Version = $args.Get(0)
}

$FlyctlInstall = $env:FLYCTL_INSTALL
$BinDir = if ($FlyctlInstall) {
  "$FlyctlInstall\bin"
} else {
  "$Home\.fly\bin"
}

$FlyctlTgz = "$BinDir\flyctl.tar.gz"
$FlyctlExe = "$BinDir\flyctl.exe"
$Target = 'Windows_x86_64'

# GitHub requires TLS 1.2
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

$FlyctlUri = if (!$Version) {
  $Response = Invoke-WebRequest 'https://github.com/superfly/flyctl/releases' -UseBasicParsing
  if ($PSVersionTable.PSEdition -eq 'Core') {
    $Response.Links |
      Where-Object { $_.href -like "/superfly/flyctl/releases/download/*/flyctl_*.*.*_${Target}.tar.gz" } |
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
      Where-Object { $_.href -like "about:/superfly/flyctl/releases/download/*/flyctl_*.*.*_${Target}.tar.gz" } |
      ForEach-Object { $_.href -replace 'about:', 'https://github.com' } |
      Select-Object -First 1
  }
} else {
  "https://github.com/denoland/deno/releases/download/${Version}/deno_${Version}_${Target}.tar.gz"
}

if (!(Test-Path $BinDir)) {
  New-Item $BinDir -ItemType Directory | Out-Null
}

Invoke-WebRequest $FlyctlUri -OutFile $FlyctlTgz -UseBasicParsing

tar xvzCf $BinDir $FlyctlTgz

# if (Get-Command Expand-Archive -ErrorAction SilentlyContinue) {
#   Expand-Archive $DenoZip -Destination $BinDir -Force
# } else {
#   if (Test-Path $DenoExe) {
#     Remove-Item $DenoExe
#   }
#   Add-Type -AssemblyName System.IO.Compression.FileSystem
#   [IO.Compression.ZipFile]::ExtractToDirectory($DenoZip, $BinDir)
# }

Remove-Item $FlyctlTgz

$User = [EnvironmentVariableTarget]::User
$Path = [Environment]::GetEnvironmentVariable('Path', $User)
if (!(";$Path;".ToLower() -like "*;$BinDir;*".ToLower())) {
  [Environment]::SetEnvironmentVariable('Path', "$Path;$BinDir", $User)
  $Env:Path += ";$BinDir"
}

Write-Output "Flyctl was installed successfully to $FlyctlExe"
Write-Output "Run 'flyctl --help' to get started"