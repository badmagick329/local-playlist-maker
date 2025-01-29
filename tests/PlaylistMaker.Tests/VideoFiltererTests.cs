using PlaylistMaker.Core;
using PlaylistMaker.Tests.Mocks;
using PlaylistMaker.View;

namespace PlaylistMaker.Tests;

public class VideoFiltererTests
{
    [Fact]
    public void Ctor_Initializes_WithMocks()
    {
        var videoFilterer = new VideoFilterer(
            new MockMusicVideoList(),
            new MockDateRangeEnquirer()
        );
        Assert.NotNull(videoFilterer);
    }

    [Fact]
    public void MusicVideoFor_Retrieves_Videos()
    {
        var musicVideoList = new MockMusicVideoList();
        var videoList = musicVideoList.VideoList();
        videoList.ForEach(videoName =>
        {
            var musicVideo = musicVideoList.MusicVideoFor(videoName);
            Assert.IsType<MusicVideo>(musicVideo);
        });
    }
}