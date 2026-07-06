using PlaylistMaker.Core;
using PlaylistMaker.Tests.Mocks;

namespace PlaylistMaker.Tests;

public class TypeOfMusicVideoTests
{
    [Theory]
    [InlineData(@"D:\Music\Areia MVs\240101 ARTMS - Birth (Remix).mp4")]
    [InlineData(@"D:\Music\Areia MVs\240102 Loossemble - Girls' Night (Areia Remix).mkv")]
    [InlineData(@"D:\Music\Areia MVs\240103 tripleS - Girls Never Die (rEmIx).webm")]
    [InlineData(@"D:\Music\Areia MVs\240104 Yves - Loop (ArEiA rEmIx).mp4")]
    public void IsRemix_ReturnsTrue_ForSupportedSuffixes(string videoPath)
    {
        var musicVideo = CreateMusicVideo(videoPath);

        Assert.True(TypeOfMusicVideo.IsRemix(musicVideo));
    }

    [Theory]
    [InlineData(@"D:\Music\Areia MVs\240105 ARTMS - Birth Remix.mp4")]
    [InlineData(@"D:\Music\Areia MVs\240106 Loossemble - Girls' Night Areia Remix.mkv")]
    [InlineData(@"D:\Music\Areia MVs\240107 Yves - Loop (Club Remix Edit).webm")]
    [InlineData(@"D:\Music\Areia MVs\240108 tripleS - Girls Never Die.mkv")]
    public void IsRemix_ReturnsFalse_ForUnsupportedSuffixes(string videoPath)
    {
        var musicVideo = CreateMusicVideo(videoPath);

        Assert.False(TypeOfMusicVideo.IsRemix(musicVideo));
    }

    [Theory]
    [InlineData(@"D:\Music\Areia MVs\240101 ARTMS - Birth (Remix).mp4")]
    [InlineData(@"D:\Music\Areia MVs\240102 Loossemble - Girls' Night (Areia Remix).mkv")]
    public void IsMusicVideo_ReturnsFalse_ForRemixVideos(string videoPath)
    {
        var musicVideo = CreateMusicVideo(videoPath);

        Assert.False(TypeOfMusicVideo.IsMusicVideo(musicVideo));
    }

    private static MusicVideo CreateMusicVideo(string videoPath)
    {
        var track = MockTrackFactory.FromVideoPath(videoPath);
        return new MusicVideo(videoPath, track);
    }
}
