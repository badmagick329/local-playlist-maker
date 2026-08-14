using System.Text;
using System.Text.Json;
using PlaylistMaker.Core;

namespace PlaylistMaker.Application;

public sealed record NormalizedHistoryEvent(
    PlaybackHistoryEvent Raw,
    string Outcome,
    double? WatchedPercent,
    double? WatchedSeconds,
    bool CountedAsPlayed
);

public sealed record PlaybackHistorySummary(
    int PlayedCount,
    int CompletedCount,
    int StoppedCount,
    int SkippedCount,
    int NotStartedCount,
    int AbandonedCount,
    DateTime? LastPlayedAtUtc,
    IReadOnlyList<NormalizedHistoryEvent> RecentEvents
)
{
    public static PlaybackHistorySummary Empty { get; } = new(0, 0, 0, 0, 0, 0, null, []);
}

public sealed class PlaybackHistoryIndex
{
    private readonly Dictionary<string, PlaybackHistorySummary> _tracks;
    private readonly Dictionary<string, PlaybackHistorySummary> _videos;

    public PlaybackHistoryIndex(
        Dictionary<string, PlaybackHistorySummary>? tracks = null,
        Dictionary<string, PlaybackHistorySummary>? videos = null,
        int invalidLineCount = 0
    )
    {
        _tracks = tracks ?? new Dictionary<string, PlaybackHistorySummary>(PathIdentity.Comparer);
        _videos = videos ?? new Dictionary<string, PlaybackHistorySummary>(PathIdentity.Comparer);
        InvalidLineCount = invalidLineCount;
    }

    public int InvalidLineCount { get; }

    public PlaybackHistorySummary ForTrack(string audioPath) =>
        _tracks.GetValueOrDefault(PathIdentity.Normalize(audioPath), PlaybackHistorySummary.Empty);

    public PlaybackHistorySummary ForVideo(string videoPath) =>
        _videos.GetValueOrDefault(PathIdentity.Normalize(videoPath), PlaybackHistorySummary.Empty);
}

public sealed class PlaybackHistoryReader(string historyPath)
{
    private static readonly HashSet<string> TerminalEvents =
        ["completed", "stopped", "skipped", "not_started", "abandoned"];
    private static readonly JsonSerializerOptions JsonOptions = new()
    {
        PropertyNameCaseInsensitive = true,
    };

    public const double CompletionPercent = 90;
    public string HistoryPath { get; } = historyPath;

    public PlaybackHistoryIndex Read()
    {
        if (!File.Exists(HistoryPath))
        {
            return new PlaybackHistoryIndex();
        }

        var invalidLines = 0;
        var terminalByEntry = new Dictionary<(string Session, string Entry), PlaybackHistoryEvent>();
        using var stream = new FileStream(
            HistoryPath,
            FileMode.Open,
            FileAccess.Read,
            FileShare.ReadWrite | FileShare.Delete
        );
        using var reader = new StreamReader(stream, Encoding.UTF8, true);
        while (reader.ReadLine() is { } line)
        {
            PlaybackHistoryEvent? historyEvent;
            try
            {
                historyEvent = JsonSerializer.Deserialize<PlaybackHistoryEvent>(line, JsonOptions);
            }
            catch (JsonException)
            {
                invalidLines++;
                continue;
            }

            if (historyEvent is null || !TerminalEvents.Contains(historyEvent.Event)
                || string.IsNullOrWhiteSpace(historyEvent.SessionId)
                || string.IsNullOrWhiteSpace(historyEvent.EntryId))
            {
                continue;
            }

            var key = (historyEvent.SessionId, historyEvent.EntryId);
            if (!terminalByEntry.TryGetValue(key, out var current)
                || historyEvent.EventAtUtc >= current.EventAtUtc)
            {
                terminalByEntry[key] = historyEvent;
            }
        }

        var normalized = terminalByEntry.Values.Select(Normalize).ToList();
        return new PlaybackHistoryIndex(
            Summarize(normalized, item => item.Raw.AudioPath),
            Summarize(normalized, item => item.Raw.VideoPath),
            invalidLines
        );
    }

    public static NormalizedHistoryEvent Normalize(PlaybackHistoryEvent historyEvent)
    {
        double? percent = historyEvent.WatchedPercent is null
            ? null
            : Math.Clamp(historyEvent.WatchedPercent.Value, 0, 100);
        var reachedEof = string.Equals(
            historyEvent.EndReason,
            "eof",
            StringComparison.OrdinalIgnoreCase
        );
        if (reachedEof)
        {
            percent = 100;
        }
        var seconds = historyEvent.WatchedSeconds;
        if (seconds is not null && historyEvent.DurationSeconds is > 0)
        {
            seconds = Math.Clamp(seconds.Value, 0, historyEvent.DurationSeconds.Value);
        }

        var outcome = reachedEof
            ? "completed"
            : historyEvent.Event == "stopped" && percent >= CompletionPercent
            ? "completed"
            : historyEvent.Event;
        var counted = outcome == "completed" || historyEvent.CountedAsPlayed == true;
        return new NormalizedHistoryEvent(historyEvent, outcome, percent, seconds, counted);
    }

    private static Dictionary<string, PlaybackHistorySummary> Summarize(
        IEnumerable<NormalizedHistoryEvent> events,
        Func<NormalizedHistoryEvent, string> pathSelector
    )
    {
        var result = new Dictionary<string, PlaybackHistorySummary>(PathIdentity.Comparer);
        foreach (var group in events
                     .Where(item => !string.IsNullOrWhiteSpace(pathSelector(item)))
                     .GroupBy(item => PathIdentity.Normalize(pathSelector(item)), PathIdentity.Comparer))
        {
            var ordered = group.OrderByDescending(item => item.Raw.EventAtUtc).ToList();
            result[group.Key] = new PlaybackHistorySummary(
                ordered.Count(item => item.CountedAsPlayed),
                ordered.Count(item => item.Outcome == "completed"),
                ordered.Count(item => item.Outcome == "stopped"),
                ordered.Count(item => item.Outcome == "skipped"),
                ordered.Count(item => item.Outcome == "not_started"),
                ordered.Count(item => item.Outcome == "abandoned"),
                ordered.Where(item => item.CountedAsPlayed)
                    .Select(item => (DateTime?)item.Raw.EventAtUtc)
                    .FirstOrDefault(),
                ordered.Take(5).ToList()
            );
        }

        return result;
    }
}
