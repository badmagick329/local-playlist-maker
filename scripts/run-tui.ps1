[CmdletBinding()]
param(
    [Parameter(ValueFromRemainingArguments = $true)]
    [string[]]$ApplicationArguments
)

$repositoryRoot = Split-Path -Parent $PSScriptRoot
$project = Join-Path $repositoryRoot 'src\PlaylistMaker.Tui\PlaylistMaker.Tui.csproj'

Push-Location $repositoryRoot
try {
    dotnet run --configuration Release --project $project -- @ApplicationArguments
}
finally {
    Pop-Location
}
