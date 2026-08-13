using System.Text.Json;
using PlaylistMaker.Application;
using PlaylistMaker.Core;

namespace PlaylistMaker.App.Tests;

public class PlaybackHistoryReaderTests : IDisposable
{
    private readonly string _directory = Path.Combine(Path.GetTempPath(), Guid.NewGuid().ToString("N"));

    [Fact]
    public void AggregatesLatestTerminalPerEntryAndNormalizesOldEvents()
    {
        Directory.CreateDirectory(_directory);
        var path = Path.Combine(_directory, "play-history.jsonl");
        var now = DateTime.UtcNow;
        var events = new[]
        {
            Event("started", "one", now.AddMinutes(-4)),
            Event("stopped", "one", now.AddMinutes(-3), 101, true),
            Event("skipped", "two", now.AddMinutes(-2), 20, false),
            Event("completed", "two", now.AddMinutes(-1), 100, true),
            Event("not_started", "three", now, null, false),
        };
        File.WriteAllLines(path, events.Select(item => JsonSerializer.Serialize(item))
            .Append("{broken json"));

        var index = new PlaybackHistoryReader(path).Read();
        var track = index.ForTrack(@"C:\audio\song.flac");

        Assert.Equal(2, track.PlayedCount);
        Assert.Equal(2, track.CompletedCount);
        Assert.Equal(0, track.SkippedCount);
        Assert.Equal(1, track.NotStartedCount);
        Assert.Equal(100, track.RecentEvents.Single(item => item.Raw.EntryId == "one").WatchedPercent);
        Assert.Equal(1, index.InvalidLineCount);
    }

    [Theory]
    [InlineData(89.9, "stopped")]
    [InlineData(90, "completed")]
    [InlineData(120, "completed")]
    public void CompletionBoundaryIsNinetyPercent(double percent, string expected)
    {
        Assert.Equal(expected, PlaybackHistoryReader.Normalize(
            Event("stopped", "one", DateTime.UtcNow, percent, true)).Outcome);
    }

    private static PlaybackHistoryEvent Event(
        string eventName,
        string entry,
        DateTime at,
        double? percent = null,
        bool? counted = null
    ) => new()
    {
        Event = eventName,
        EventAtUtc = at,
        SessionId = "session",
        EntryId = entry,
        AudioPath = @"C:\audio\song.flac",
        VideoPath = $@"C:\video\{entry}.mkv",
        WatchedPercent = percent,
        WatchedSeconds = percent is null ? null : 200,
        DurationSeconds = percent is null ? null : 180,
        CountedAsPlayed = counted,
    };

    public void Dispose()
    {
        if (Directory.Exists(_directory)) Directory.Delete(_directory, true);
    }
}
