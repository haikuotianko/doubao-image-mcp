# 打包全平台 Release 资产。用法：pwsh -File scripts\build-release.ps1 -Version v1.0.0
param([string]$Version = 'v1.0.0')

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

$dist = Join-Path $root 'build\dist'
$pack = Join-Path $dist 'pack'
Remove-Item $pack -Recurse -Force -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force $pack | Out-Null

function New-Package {
    param(
        [string]$Source,     # 构建产物路径
        [string]$Entry,      # 压缩包里的可执行文件名
        [string]$Suffix      # 平台后缀，如 windows-x64
    )
    $dir = Join-Path $pack $Suffix
    New-Item -ItemType Directory -Force $dir | Out-Null
    Copy-Item $Source (Join-Path $dir $Entry)
    Copy-Item 'README.md', 'LICENSE' $dir
    $zip = Join-Path $dist "doubao-image-mcp-$Version-$Suffix.zip"
    if (Test-Path $zip) { Remove-Item $zip -Force }
    Compress-Archive -Path (Join-Path $dir '*') -DestinationPath $zip -CompressionLevel Optimal
    $mb = [math]::Round((Get-Item $zip).Length / 1MB, 2)
    Write-Host ("  {0,-34} {1,6} MB" -f (Split-Path $zip -Leaf), $mb)
}

Write-Host "== Windows（含 GUI，Wails 构建）"
# 资产名用大家熟悉的叫法：amd64 → x64，386 → x86
$winTargets = @(
    @{ arch = 'amd64'; suffix = 'windows-x64' },
    @{ arch = '386';   suffix = 'windows-x86' },
    @{ arch = 'arm64'; suffix = 'windows-arm64' }
)

foreach ($t in $winTargets) {
    & wails build -platform "windows/$($t.arch)" -o doubao-image-mcp.exe | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "windows/$($t.arch) 构建失败" }
    New-Package (Join-Path $root 'build\bin\doubao-image-mcp.exe') 'doubao-image-mcp.exe' $t.suffix
}

Write-Host "== macOS / Linux（仅命令行：MCP + -set-session，无 GUI）"
# 这些平台的 GUI 需要 cgo（WKWebView / WebKitGTK），Windows 上无法交叉编译，
# 所以发的是纯 Go 的 CLI 构建，MCP 功能完整。

$cliTargets = @(
    @{ os = 'darwin'; arch = 'arm64'; suffix = 'macos-arm64-cli' },
    @{ os = 'darwin'; arch = 'amd64'; suffix = 'macos-x64-cli' },
    @{ os = 'linux';  arch = 'amd64'; suffix = 'linux-x64-cli' },
    @{ os = 'linux';  arch = 'arm64'; suffix = 'linux-arm64-cli' }
)

foreach ($t in $cliTargets) {
    $out = Join-Path $root ("build\dist\.tmp-{0}-{1}" -f $t.os, $t.arch)
    $env:GOOS = $t.os
    $env:GOARCH = $t.arch
    $env:CGO_ENABLED = '0'
    & go build -trimpath -ldflags '-s -w' -o $out .
    $code = $LASTEXITCODE
    Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED -ErrorAction SilentlyContinue
    if ($code -ne 0) { throw "$($t.os)/$($t.arch) 构建失败" }
    New-Package $out 'doubao-image-mcp' $t.suffix
    Remove-Item $out -Force
}

Write-Host "== 恢复本地开发版为 windows/amd64"
# 上面最后一个 wails 构建是 arm64，这里重建一次，保证 build\bin 里是常用的 x64
& wails build -platform "windows/amd64" -o doubao-image-mcp.exe | Out-Null
if ($LASTEXITCODE -ne 0) { throw "windows/amd64 重建失败" }

Write-Host "== 结束，产物在 build\dist\"
