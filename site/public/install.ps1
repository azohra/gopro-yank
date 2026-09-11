param(
  [switch]$NoLaunch
)

$ErrorActionPreference = "Stop"
$releaseUrl = if ($env:GOPRO_YANK_RELEASE_URL) { $env:GOPRO_YANK_RELEASE_URL } else { "https://github.com/azohra/gopro-yank/releases/latest/download" }
$installDir = if ($env:GOPRO_YANK_INSTALL_DIR) { $env:GOPRO_YANK_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA "Programs\gopro-yank" }
$architecture = if ($env:PROCESSOR_ARCHITECTURE -match "ARM64") { "arm64" } else { "amd64" }
$asset = "gopro-yank_windows_$architecture.zip"
$temporaryDir = Join-Path ([IO.Path]::GetTempPath()) ("gopro-yank-install-" + [Guid]::NewGuid().ToString("N"))

try {
  New-Item -ItemType Directory -Path $temporaryDir | Out-Null
  $archive = Join-Path $temporaryDir $asset
  $checksums = Join-Path $temporaryDir "checksums.txt"

  Write-Host "Downloading GoPro Yank for windows/$architecture..."
  Invoke-WebRequest -UseBasicParsing -Uri "$releaseUrl/$asset" -OutFile $archive
  Invoke-WebRequest -UseBasicParsing -Uri "$releaseUrl/checksums.txt" -OutFile $checksums

  $line = Get-Content $checksums | Where-Object { $_ -match "\s$([regex]::Escape($asset))$" } | Select-Object -First 1
  if (-not $line) { throw "The release checksum list does not include this build." }
  $expected = ($line -split "\s+")[0].ToLowerInvariant()
  $actual = (Get-FileHash -Algorithm SHA256 $archive).Hash.ToLowerInvariant()
  if ($actual -ne $expected) { throw "Checksum verification failed. Nothing was installed." }

  $expanded = Join-Path $temporaryDir "expanded"
  Expand-Archive -Path $archive -DestinationPath $expanded
  $source = Join-Path $expanded "gopro-yank.exe"
  if (-not (Test-Path $source)) { throw "The release archive did not contain gopro-yank.exe." }

  New-Item -ItemType Directory -Force -Path $installDir | Out-Null
  Copy-Item -Force $source (Join-Path $installDir "gopro-yank.exe")

  $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
  $entries = @($userPath -split ";" | Where-Object { $_ })
  if ($entries -notcontains $installDir) {
    [Environment]::SetEnvironmentVariable("Path", (($entries + $installDir) -join ";"), "User")
  }

  $executable = Join-Path $installDir "gopro-yank.exe"
  Write-Host "Installed GoPro Yank at $executable"
  if (-not $NoLaunch) { & $executable }
}
finally {
  if (Test-Path $temporaryDir) { Remove-Item -Recurse -Force $temporaryDir }
}
