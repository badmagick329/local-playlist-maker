[CmdletBinding()]
param(
    [switch]$EnableHistory,

    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$ApplicationArguments
)

$repositoryRoot = Split-Path -Parent $PSScriptRoot
$goProject = Join-Path $repositoryRoot 'src\PlaylistMaker.Charm'
$bridgeProject = Join-Path $repositoryRoot 'src\PlaylistMaker.Bridge\PlaylistMaker.Bridge.csproj'
$outputDirectory = Join-Path $repositoryRoot 'artifacts\charm\cache'
$config = Join-Path $repositoryRoot 'config.yaml'

New-Item -ItemType Directory -Force -Path $outputDirectory | Out-Null

function Get-SourceFingerprint {
    param(
        [System.IO.FileInfo[]]$Sources
    )

    $description = $Sources |
        Sort-Object FullName |
        ForEach-Object { "$($_.FullName)|$($_.Length)|$($_.LastWriteTimeUtc.Ticks)" }
    $bytes = [System.Text.Encoding]::UTF8.GetBytes(($description -join "`n"))
    $algorithm = [System.Security.Cryptography.SHA256]::Create()
    try {
        return [Convert]::ToHexString($algorithm.ComputeHash($bytes)).Substring(0, 12).ToLowerInvariant()
    }
    finally {
        $algorithm.Dispose()
    }
}

$bridgeSources = @(
    Get-ChildItem -Path (Join-Path $repositoryRoot 'src\PlaylistMaker.Bridge'), (Join-Path $repositoryRoot 'src\PlaylistMaker.App') -Recurse -File |
        Where-Object { $_.Extension -in '.cs', '.csproj' }
    Get-Item -LiteralPath (Join-Path $repositoryRoot 'mpv-scripts\playlistmaker-history.lua')
)
$bridgeFingerprint = Get-SourceFingerprint -Sources $bridgeSources
$bridgeDirectory = Join-Path $outputDirectory "bridge-$bridgeFingerprint"
$bridgeExecutable = Join-Path $bridgeDirectory 'PlaylistMaker.Bridge.exe'
if (-not (Test-Path -LiteralPath $bridgeExecutable)) {
    Write-Host 'Building PlaylistMaker bridge...'
    dotnet publish $bridgeProject --configuration Release --self-contained false --output $bridgeDirectory
    if ($LASTEXITCODE -ne 0) {
        throw 'Unable to build PlaylistMaker bridge.'
    }
}

$goSources = @(
    Get-ChildItem -Path $goProject -Recurse -File |
        Where-Object { $_.Extension -eq '.go' -or $_.Name -in 'go.mod', 'go.sum' }
)
$goFingerprint = Get-SourceFingerprint -Sources $goSources
$executable = Join-Path $outputDirectory "playlistmaker-charm-$goFingerprint.exe"
if (-not (Test-Path -LiteralPath $executable)) {
    Write-Host 'Building PlaylistMaker Charm...'
    Push-Location $goProject
    try {
        go build -trimpath -ldflags '-s -w' -o $executable .\cmd\playlistmaker-charm
        if ($LASTEXITCODE -ne 0) {
            throw 'Unable to build PlaylistMaker Charm.'
        }
    }
    finally {
        Pop-Location
    }
}

Push-Location $repositoryRoot
try {
    $arguments = @('--bridge', $bridgeExecutable, '--config', $config)
    if (-not $EnableHistory) {
        $arguments += '--disable-history'
    }
    $arguments += $ApplicationArguments
    & $executable $arguments
}
finally {
    Pop-Location
}
