using System.Text.Json;
using PlaylistMaker.Core;
using PlaylistMaker.Infrastructure;

namespace PlaylistMaker.Tests;

public class PlaybackHistoryServiceTests : IDisposable
{
    private readonly string _dataDirectory = Path.Combine(Path.GetTempPath(), Guid.NewGuid().ToString("N"));

    [Fact]
    public void Config_HistoryIsDisabledByDefaultWithAFiftyPercentThreshold()
    {
        var config = new Config();

        Assert.False(config.PlaybackHistoryEnabled);
        Assert.Equal(50, config.PlaybackHistoryMinimumWatchedPercent);
    }

    [Fact]
    public void CreateSession_WritesEntriesWithMetadataAndUniqueIds()
    {
        var history = new PlaybackHistoryService(_dataDirectory, 50);
        var video = CreateVideo();

        var first = history.CreateSession([video], "interactive-selection");
        var second = history.CreateSession([video], "interactive-selection");

        var entry = Assert.Single(first.Entries);
        Assert.NotEqual(first.SessionId, second.SessionId);
        Assert.False(string.IsNullOrWhiteSpace(entry.EntryId));
        Assert.Equal(video.FilePath, entry.VideoPath);
        Assert.Equal(video.Track.FilePath, entry.AudioPath);
        Assert.Equal("Artist", entry.Artist);
        Assert.Equal("Title", entry.Title);
        Assert.Equal(0, entry.PlaylistPosition);
        Assert.Equal(1, entry.PlaylistSize);
        Assert.True(File.Exists(Path.Combine(_dataDirectory, "playback-sessions", $"{first.SessionId}.json")));
    }

    [Fact]
    public void MpvArgumentsFor_IncludeTheSessionAndHistorySettings()
    {
        var history = new PlaybackHistoryService(_dataDirectory, 50);
        var session = history.CreateSession([CreateVideo()], "interactive-selection");

        var arguments = history.MpvArgumentsFor(session);

        Assert.Contains(arguments, argument => argument == $"--script-opt=playlistmaker_history-session_id={session.SessionId}");
        Assert.Contains(arguments, argument => argument.EndsWith("minimum_watched_percent=50"));
        Assert.Contains(arguments, argument => argument.Contains("manifest_path="));
        Assert.Contains(arguments, argument => argument.Contains("history_path="));
    }

    [Fact]
    public void RecoverIncompleteSessions_WritesAbandonedAndNotStartedOnce()
    {
        var history = new PlaybackHistoryService(_dataDirectory, 50);
        var session = history.CreateSession([CreateVideo(), CreateVideo("second")], "text-playlist");
        var startedEntry = session.Entries[0];
        AppendHistoryEvent(new PlaybackHistoryEvent
        {
            Event = "started",
            EventAtUtc = DateTime.UtcNow,
            SessionId = session.SessionId,
            EntryId = startedEntry.EntryId,
        });

        history.RecoverIncompleteSessions();
        history.RecoverIncompleteSessions();

        var events = ReadHistory().Where(e => e.SessionId == session.SessionId).ToList();
        Assert.Contains(events, e => e.Event == "abandoned" && e.EntryId == startedEntry.EntryId);
        Assert.Contains(events, e => e.Event == "not_started" && e.EntryId == session.Entries[1].EntryId);
        Assert.Equal(3, events.Count);
        Assert.False(File.Exists(Path.Combine(_dataDirectory, "playback-sessions", $"{session.SessionId}.json")));
    }

    [Theory]
    [InlineData(49.9, false)]
    [InlineData(50, true)]
    [InlineData(100, true)]
    public void ThresholdBoundary_IsConfiguredAsExpected(double watchedPercent, bool countedAsPlayed)
    {
        Assert.Equal(countedAsPlayed, watchedPercent >= 50);
    }

    private MusicVideo CreateVideo(string suffix = "")
    {
        var track = new Track(1, "Artist", $"Title{suffix}", new ReleaseDate(2024, 1, 1), $"C:\\music\\audio{suffix}.flac");
        return new MusicVideo($"C:\\videos\\240101 video{suffix}.mp4", track);
    }

    private void AppendHistoryEvent(PlaybackHistoryEvent historyEvent)
    {
        var options = new JsonSerializerOptions { PropertyNamingPolicy = JsonNamingPolicy.CamelCase };
        File.AppendAllText(
            Path.Combine(_dataDirectory, "play-history.jsonl"),
            JsonSerializer.Serialize(historyEvent, options) + Environment.NewLine
        );
    }

    private IEnumerable<PlaybackHistoryEvent> ReadHistory()
    {
        var options = new JsonSerializerOptions { PropertyNameCaseInsensitive = true };
        return File.ReadLines(Path.Combine(_dataDirectory, "play-history.jsonl"))
            .Select(line => JsonSerializer.Deserialize<PlaybackHistoryEvent>(line, options)!)
            .ToList();
    }

    public void Dispose()
    {
        if (Directory.Exists(_dataDirectory))
        {
            Directory.Delete(_dataDirectory, true);
        }
    }
}
