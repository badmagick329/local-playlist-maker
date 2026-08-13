using PlaylistMaker.Application;
using PlaylistMaker.Core;

namespace PlaylistMaker.App.Tests;

public class QueuePlaybackServiceTests
{
    [Theory]
    [InlineData(true, 0)]
    [InlineData(false, 1)]
    public void ClearsQueueOnlyAfterSuccessfulLaunch(bool succeeds, int expectedQueueCount)
    {
        var queue = new PlaylistDraft();
        queue.Add(Variant());
        var service = new QueuePlaybackService(new PlaybackPlanner(new Random(1)), new StubCoordinator(succeeds));

        var result = service.Launch(queue, new PlaybackOptions());

        Assert.Equal(succeeds, result.Launch.Succeeded);
        Assert.Equal(expectedQueueCount, queue.Items.Count);
    }

    private static VideoVariant Variant()
    {
        var track = new Track(1, "Artist", "Title", new ReleaseDate(2024, 1, 1), @"C:\audio\song.flac");
        var video = new MusicVideo(@"C:\video\240101 Artist - Title.mkv", track);
        return new VideoVariant(PathIdentity.Normalize(video.FilePath), video, VideoCategory.MusicVideo, DateTime.MinValue);
    }

    private sealed class StubCoordinator(bool succeeds) : IPlaybackCoordinator
    {
        public PlaybackLaunchResult Launch(PlaybackRequest request) =>
            new(succeeds, succeeds ? null : "failed");
    }
}
