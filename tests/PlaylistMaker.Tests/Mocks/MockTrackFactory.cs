using PlaylistMaker.Core;

namespace PlaylistMaker.Tests.Mocks;

static class MockTrackFactory
{
    public static Track FromVideoPath(string videoPath)
    {
        var videoExtension = Path.GetExtension(videoPath);
        var pathAsFlac = videoPath.Replace(videoExtension, ".flac");
        return new Track(
            1, "Artist", "Title", ReleaseDateFactory.FromVideoPath(videoPath), pathAsFlac
        );
    }
}