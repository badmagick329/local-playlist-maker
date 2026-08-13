namespace PlaylistMaker.Core;

public class PlaybackSessionManifest
{
    public int SchemaVersion { get; init; } = 1;
    public string SessionId { get; init; } = string.Empty;
    public DateTime RequestedAtUtc { get; init; }
    public int? MpvProcessId { get; set; }
    public List<PlaybackSessionEntry> Entries { get; init; } = [];
}

public class PlaybackSessionEntry
{
    public string EntryId { get; init; } = string.Empty;
    public int PlaylistPosition { get; init; }
    public int PlaylistSize { get; init; }
    public string SelectionSource { get; init; } = string.Empty;
    public string VideoPath { get; init; } = string.Empty;
    public string AudioPath { get; init; } = string.Empty;
    public string Artist { get; init; } = string.Empty;
    public string Title { get; init; } = string.Empty;
}

public class PlaybackHistoryEvent
{
    public int SchemaVersion { get; init; } = 2;
    public string Event { get; init; } = string.Empty;
    public DateTime EventAtUtc { get; init; }
    public string SessionId { get; init; } = string.Empty;
    public string EntryId { get; init; } = string.Empty;
    public int PlaylistPosition { get; init; }
    public int PlaylistSize { get; init; }
    public string SelectionSource { get; init; } = string.Empty;
    public string VideoPath { get; init; } = string.Empty;
    public string AudioPath { get; init; } = string.Empty;
    public string Artist { get; init; } = string.Empty;
    public string Title { get; init; } = string.Empty;
    public double? DurationSeconds { get; init; }
    public double? WatchedSeconds { get; init; }
    public double? WatchedPercent { get; init; }
    public double? FinalPositionSeconds { get; init; }
    public string? EndReason { get; init; }
    public bool? CountedAsPlayed { get; init; }
}
