using PlaylistMaker.Application;
using PlaylistMaker.Core;

namespace PlaylistMaker.App.Tests;

public class PlaylistDraftAndPlanningTests
{
    [Fact]
    public void DraftIsUniqueOrderedAndMovable()
    {
        var first = Variant("first", "track-a");
        var second = Variant("second", "track-b");
        var draft = new PlaylistDraft();

        Assert.True(draft.Add(first));
        Assert.False(draft.Add(first));
        Assert.True(draft.Add(second));
        Assert.True(draft.Move(1, -1));
        Assert.Equal([second, first], draft.Items);
        Assert.False(draft.Toggle(second));
        Assert.Equal([first], draft.Items);
        draft.RemoveAt(0);
        Assert.True(draft.Add(first));
        draft.Clear();
        Assert.True(draft.Add(first));
    }

    [Fact]
    public void PlannerAppliesOnePerTrackRepeatShuffleThenMaximum()
    {
        var variants = new[]
        {
            Variant("a1", "track-a"),
            Variant("a2", "track-a"),
            Variant("b1", "track-b"),
        };
        var request = new PlaybackPlanner(new Random(1)).Create(
            variants,
            new PlaybackOptions(Shuffle: true, MaximumItems: 3, RepeatEach: 2, OneVideoPerTrack: true)
        );

        Assert.Equal(3, request.Videos.Count);
        Assert.Equal(2, request.Videos.Select(video => video.Track.FilePath).Distinct(PathIdentity.Comparer).Count());
    }

    private static VideoVariant Variant(string video, string audio)
    {
        var track = new Track(1, "Artist", audio, new ReleaseDate(2024, 1, 1), $@"C:\audio\{audio}.flac");
        var musicVideo = new MusicVideo($@"C:\videos\240101 Artist - {video}.mkv", track);
        return new VideoVariant(PathIdentity.Normalize(musicVideo.FilePath), musicVideo, VideoCategory.MusicVideo, DateTime.MinValue);
    }
}
