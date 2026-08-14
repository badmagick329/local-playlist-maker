[CmdletBinding()]
param(
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$ApplicationArguments
)

$repositoryRoot = Split-Path -Parent $PSScriptRoot
$goProject = Join-Path $repositoryRoot 'src\PlaylistMaker.Charm'
$bridgeProject = Join-Path $repositoryRoot 'src\PlaylistMaker.Bridge\PlaylistMaker.Bridge.csproj'
$outputDirectory = Join-Path $repositoryRoot 'artifacts\charm'
$bridgeDirectory = Join-Path $outputDirectory 'bridge'
$bridgeExecutable = Join-Path $bridgeDirectory 'PlaylistMaker.Bridge.exe'
$executable = Join-Path $outputDirectory 'playlistmaker-charm.exe'
$config = Join-Path $repositoryRoot 'config.yaml'

New-Item -ItemType Directory -Force -Path $outputDirectory | Out-Null

dotnet publish $bridgeProject --configuration Release --self-contained false --output $bridgeDirectory
if ($LASTEXITCODE -ne 0) {
    throw 'Unable to build PlaylistMaker bridge.'
}

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

Push-Location $repositoryRoot
try {
    & $executable --bridge $bridgeExecutable --config $config @ApplicationArguments
}
finally {
    Pop-Location
}
