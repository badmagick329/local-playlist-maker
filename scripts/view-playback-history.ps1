param(
    [string]$SessionId,
    [string]$HistoryPath = (Join-Path $PSScriptRoot '..\data\play-history.jsonl')
)

$historyFile = [System.IO.Path]::GetFullPath($HistoryPath)

if (-not (Get-Command jq -ErrorAction SilentlyContinue)) {
    throw 'jq is required. Install it with: scoop install jq'
}

if (-not (Test-Path -LiteralPath $historyFile)) {
    throw "Playback history file not found: $historyFile"
}

& jq --color-output --arg sessionId $SessionId '
    select($sessionId == "" or .sessionId == $sessionId)
' $historyFile
