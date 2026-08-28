[CmdletBinding(PositionalBinding = $false)]
param(
    [switch]$DisableHistory,
	[switch]$AllowUntrackedPlayback,

    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$ApplicationArguments
)

$repositoryRoot = Split-Path -Parent $PSScriptRoot
$goProject = Join-Path $repositoryRoot 'src\PlaylistMaker.Charm'
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

$goSources = @(
    Get-ChildItem -Path $goProject -Recurse -File |
        Where-Object { $_.Extension -in '.go', '.lua' -or $_.Name -in 'go.mod', 'go.sum' }
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
	$arguments = @('--config', $config)
    if ($DisableHistory) {
        $arguments += '--disable-history'
    }
	if ($AllowUntrackedPlayback) {
		$arguments += '--allow-untracked-playback'
	}
    $arguments += $ApplicationArguments
    & $executable $arguments
    if ($LASTEXITCODE -ne 0) {
        exit $LASTEXITCODE
    }
}
finally {
    Pop-Location
}
