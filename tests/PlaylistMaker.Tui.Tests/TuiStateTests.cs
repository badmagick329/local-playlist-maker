using PlaylistMaker.Application;
using PlaylistMaker.Core;
using PlaylistMaker.Tui;

namespace PlaylistMaker.Tui.Tests;

public class TuiStateTests : IDisposable
{
    private readonly string _directory = Path.Combine(Path.GetTempPath(), Guid.NewGuid().ToString("N"));

    [Fact]
    public void StartsWithOfficialMusicVideosOnlyAndExpandsVariants()
    {
        var state = CreateState();

        var result = Assert.Single(state.Results);
        Assert.All(result.EligibleVariants, variant => Assert.Equal(VideoCategory.MusicVideo, variant.Category));
        Assert.Single(state.Rows);

        state.ToggleCategory(VideoCategory.Performance);
        state.ToggleExpansion(0);

        Assert.Equal(3, state.Rows.Count);
        Assert.Equal(2, state.Rows.Count(row => row.Variant is not null));
    }

    [Fact]
    public void QueueSurvivesSearchAndFilterChanges()
    {
        var state = CreateState();
        state.ToggleQueue(0);

        state.SetSearch("nothing can match this phrase");
        state.ToggleCategory(VideoCategory.Performance);

        Assert.Single(state.Queue.Items);
    }

    [Fact]
    public void VisibleTracksQueueOneDefaultWhileMatchingVideosQueuesEveryVariant()
    {
        var state = CreateState();
        state.ToggleCategory(VideoCategory.Performance);

        state.QueueVisibleTracks();
        Assert.Single(state.Queue.Items);
        state.Queue.Clear();

        state.QueueMatchingVideos();
        Assert.Equal(2, state.Queue.Items.Count);
    }

    private TuiState CreateState()
    {
        Directory.CreateDirectory(_directory);
        var audio = @"C:\audio\song.flac";
        var map = new Dictionary<string, string>
        {
            [@"C:\videos\240101 Artist - Song.mkv"] = audio,
            [@"C:\videos\250101 Artist - Song Performance.mkv"] = audio,
        };
        var catalog = new MediaLibraryCatalog(new StubReader(audio), map);
        return new TuiState(catalog, new PlaybackHistoryReader(Path.Combine(_directory, "history.jsonl")));
    }

    private sealed class StubReader(string audioPath) : IVorbisReader
    {
        public VorbisData? VorbisDataFor(string filePath) =>
            new(audioPath, "Artist", "Song", "2024-01-01", 1, "now");
        public List<string> GetAllFilePaths() => [audioPath];
    }

    public void Dispose()
    {
        if (Directory.Exists(_directory)) Directory.Delete(_directory, true);
    }
}
