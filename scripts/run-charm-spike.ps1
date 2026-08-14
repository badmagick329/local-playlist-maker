[CmdletBinding()]
param(
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$ApplicationArguments
)

$repositoryRoot = Split-Path -Parent $PSScriptRoot
$project = Join-Path $repositoryRoot 'src\PlaylistMaker.Charm'
$outputDirectory = Join-Path $repositoryRoot 'artifacts\charm-spike'
$executable = Join-Path $outputDirectory 'playlistmaker-charm.exe'

New-Item -ItemType Directory -Force -Path $outputDirectory | Out-Null

Push-Location $project
try {
    go build -trimpath -ldflags '-s -w' -o $executable .\cmd\playlistmaker-charm
    if ($LASTEXITCODE -ne 0) {
        throw 'Unable to build PlaylistMaker Charm performance spike.'
    }

    & $executable @ApplicationArguments
}
finally {
    Pop-Location
}
