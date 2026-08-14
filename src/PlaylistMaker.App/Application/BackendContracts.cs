using PlaylistMaker.Core;

namespace PlaylistMaker.Application;

// These are deliberately transport-neutral. During the migration the bridge serializes
// them, while the Go backend will eventually produce the same results directly.
public sealed record BackendLibrarySnapshot(int SchemaVersion, IReadOnlyList<BackendTrack> Tracks);

public sealed record BackendTrack(
    string Id,
    string Artist,
    string Title,
    string ReleaseDate,
    BackendHistory History,
    IReadOnlyList<BackendVariant> Variants
);

public sealed record BackendVariant(
    string Id,
    string VideoPath,
    string AudioPath,
    string FileName,
    string Category,
    string VideoDate,
    DateTime ModifiedAtUtc,
    BackendHistory History
);

public sealed record BackendHistory(
    int PlayedCount,
    int CompletedCount,
    int StoppedCount,
    int SkippedCount,
    DateTime? LastPlayedAtUtc
);

public sealed record BackendPlaybackOptions(
    bool Shuffle = false,
    int MaximumItems = 0,
    int RepeatEach = 1,
    bool OneVideoPerTrack = false
);

public sealed record BackendPlaybackRequest(
    IReadOnlyList<string> VideoIds,
    BackendPlaybackOptions Options
);

public sealed record BackendPlaybackResult(
    bool Succeeded,
    int PlannedVideoCount,
    string? Error = null,
    int? MpvProcessId = null
);

public static class BackendSnapshotFactory
{
    public static BackendLibrarySnapshot Create(
        MediaLibraryCatalog catalog,
        PlaybackHistoryIndex history
    ) => new(
        1,
        catalog.Tracks.Select(track => new BackendTrack(
            track.Id,
            track.Track.Artist,
            track.Track.Title,
            track.Track.Date.AsString,
            ToHistory(history.ForTrack(track.Track.FilePath)),
            track.Variants.Select(variant => new BackendVariant(
                variant.Id,
                variant.VideoPath,
                variant.AudioPath,
                variant.FileName,
                CategoryLabel(variant.Category),
                variant.VideoDate.AsString,
                variant.ModifiedAtUtc,
                ToHistory(history.ForVideo(variant.VideoPath))
            )).ToList()
        )).ToList()
    );

    private static BackendHistory ToHistory(PlaybackHistorySummary history) => new(
        history.PlayedCount,
        history.CompletedCount,
        history.StoppedCount,
        history.SkippedCount,
        history.LastPlayedAtUtc
    );

    private static string CategoryLabel(VideoCategory category) => category switch
    {
        VideoCategory.MusicVideo => "Music Video",
        VideoCategory.BandLive => "Band Live",
        VideoCategory.BeOriginal => "Be Original",
        VideoCategory.LiveAudio => "Live Audio",
        VideoCategory.MusicShow => "Music Show",
        _ => category.ToString(),
    };
}
