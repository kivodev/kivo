# Builds the Windows installer and portable zip into server\bin\windows:
# Kivo-Setup-<version>.exe and kivo-<version>-windows-amd64.zip. Needs Go,
# Node and Inno Setup 6. The release workflow runs it on a Windows runner.
#
#   .\server\packaging\windows\build.ps1 -Version 0.3.1
param([Parameter(Mandatory)][string]$Version)

$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $true
$server = Resolve-Path "$PSScriptRoot\..\.."
$out = Join-Path $server 'bin\windows'
$stage = Join-Path $out 'stage'

# The same jellyfin-ffmpeg as the image: the version is the Dockerfile's
# JELLYFIN_FFMPEG, and this is its portable Windows zip's hash. Bumping
# the Dockerfile's version needs a new hash here.
$ffmpegSha256 = '3691f8b2d4c39c511b5faff1c8c4687bd782dac823e63ba69f0755f7ce1af9ce'
$ffmpegVersion = (Select-String -Path "$server\Dockerfile" -Pattern '^ARG JELLYFIN_FFMPEG=(.+)$').Matches[0].Groups[1].Value

if (Test-Path $out) { Remove-Item -Recurse -Force $out }
New-Item -ItemType Directory -Force "$stage\ffmpeg" | Out-Null

Push-Location "$server\web"
try {
    npm ci
    npm run build
} finally {
    Pop-Location
}

# kivow.exe has no console, so it runs in the tray. kivo.exe is the same
# server for a terminal, and runs the admin commands.
$env:CGO_ENABLED = '0'
$env:GOOS = 'windows'
$env:GOARCH = 'amd64'
Push-Location $server
try {
    go build -trimpath -tags webui -ldflags "-s -w -X main.version=$Version" -o "$stage\kivo.exe" ./cmd/kivo
    go build -trimpath -tags webui -ldflags "-s -w -H windowsgui -X main.version=$Version" -o "$stage\kivow.exe" ./cmd/kivo
} finally {
    Pop-Location
}

$zip = Join-Path $out 'jellyfin-ffmpeg.zip'
Invoke-WebRequest -Uri "https://github.com/jellyfin/jellyfin-ffmpeg/releases/download/v$ffmpegVersion/jellyfin-ffmpeg_${ffmpegVersion}_portable_win64-clang-gpl.zip" -OutFile $zip
if ((Get-FileHash -Algorithm SHA256 $zip).Hash -ne $ffmpegSha256) {
    throw "jellyfin-ffmpeg $ffmpegVersion doesn't match its pinned hash"
}
Expand-Archive -Path $zip -DestinationPath "$stage\ffmpeg"
Remove-Item $zip

# The notices, as make notices writes them for the image: packaging's, with
# this jellyfin-ffmpeg, each Go module in the Windows build with its license
# (go-licenses, with GOOS still windows, so the tray's modules are in it),
# go-sqlite3-wasm's MIT-0, which go-licenses doesn't know, Go's own, and
# CLDR's Unicode license. The texts are rewritten with Windows line endings,
# for the installer's pages and Notepad.
$rule = '=' * 80
$sqliteWasm = 'github.com/ncruces/go-sqlite3-wasm/v6'
Push-Location $server
try {
    $modules = go run github.com/google/go-licenses/v2@v2.0.1 report ./cmd/kivo --ignore github.com/mmcdole/kivo --ignore $sqliteWasm --template packaging/go-licenses.tpl
    $sqliteWasmName = go list -m $sqliteWasm
    $sqliteWasmDir = go list -m -f '{{.Dir}}' $sqliteWasm
    $goRoot = go env GOROOT
    $goVersion = go env GOVERSION
} finally {
    Pop-Location
}
$notices = @(
    (Get-Content "$server\packaging\THIRD-PARTY-NOTICES.txt") -replace '\{ffmpeg\}', $ffmpegVersion
    $modules
    '', $rule, "$sqliteWasmName (MIT-0)", ''
    Get-Content "$sqliteWasmDir\LICENSE"
    '', $rule, "The Go standard library, $goVersion (BSD-3-Clause)", ''
    Get-Content "$goRoot\LICENSE"
    '', $rule, 'Unicode CLDR', ''
    Get-Content "$server\..\shared\credits\unicode-license.txt"
)
Get-Content "$server\..\LICENSE" | Set-Content "$stage\LICENSE.txt"
$notices | Set-Content "$stage\THIRD-PARTY-NOTICES.txt"
Copy-Item "$server\cmd\kivo\kivo.ico" $stage

# Signing goes here: kivo.exe and kivow.exe now, and the installer through iscc's SignTool.

Compress-Archive -Path "$stage\*" -DestinationPath "$out\kivo-$Version-windows-amd64.zip"
& "${env:ProgramFiles(x86)}\Inno Setup 6\ISCC.exe" /Q "/DVersion=$Version" "/DStage=$stage" "/O$out" "$PSScriptRoot\kivo.iss"
Remove-Item -Recurse -Force $stage
