# 生成官网用的 docs/latest.json（发版后跑一次，然后提交）。
#
# 为什么需要它：官网原来直接调 api.github.com 取最新 Release，未认证请求每小时只有 60 次，
# 用户一多（或同一出口 IP 的访客多）就会 403，下载列表退化成「前往 Releases」。
# 改成读同源的静态 JSON：不限流、加载快、不依赖第三方接口。
#
# 用法：
#   powershell -File scripts\update-latest.ps1              # 取 GitHub 上最新 Release
#   powershell -File scripts\update-latest.ps1 -Tag v1.2.0  # 指定版本

param(
    [string]$Tag = '',
    [string]$Repo = 'haikuotianko/doubao-image-mcp'
)

$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $PSScriptRoot

$uri = if ($Tag) {
    "https://api.github.com/repos/$Repo/releases/tags/$Tag"
} else {
    "https://api.github.com/repos/$Repo/releases/latest"
}

$headers = @{ 'User-Agent' = 'doubao-image-mcp'; 'Accept' = 'application/vnd.github+json' }

# 未认证请求每小时只有 60 次，本机经常被限流；有 git 凭据就用它（不会打印 token）。
try {
    $cred = ("protocol=https`nhost=github.com`n`n" | git credential fill) 2>$null
    $token = ($cred | Where-Object { $_ -like 'password=*' }) -replace '^password=',''
    if ($token) {
        $headers['Authorization'] = "Bearer $token"
        Write-Host '使用 git 凭据访问 GitHub API'
    }
} catch {
    Write-Host '没有可用的 git 凭据，改用匿名请求'
}

$rel = Invoke-RestMethod -Uri $uri -Headers $headers

$object = [ordered]@{
    tag       = $rel.tag_name
    name      = $rel.name
    published = $rel.published_at
    url       = $rel.html_url
    assets    = @(
        $rel.assets | ForEach-Object {
            [ordered]@{
                name = $_.name
                size = $_.size
                url  = $_.browser_download_url
            }
        }
    )
}

$json = $object | ConvertTo-Json -Depth 5
$out = Join-Path $root 'docs\latest.json'
[System.IO.File]::WriteAllText($out, $json, (New-Object System.Text.UTF8Encoding($false)))

Write-Host ("已写入 {0}（{1}，{2} 个资产）" -f $out, $rel.tag_name, $rel.assets.Count)
