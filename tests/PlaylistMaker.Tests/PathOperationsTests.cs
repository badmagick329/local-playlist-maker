using PlaylistMaker.Common;

namespace PlaylistMaker.Tests;

public class PathOperationsTests
{
    [Theory]
    [MemberData(nameof(FixFlacPathData))]
    public void FixFlacPath_PathIsFixed_WhenPathContainsForwardSlashes(
        string path,
        string expected
    )
    {
        var result = PathOperations.FixFlacPath(path);
        Assert.Equal(expected, result);
    }

    public static TheoryData<string, string> FixFlacPathData()
    {
        return new TheoryData<string, string>
        {
            { "/Library/Artist/Album/01 - Song.flac", @"\Library\Artist/Album/01 - Song.flac" },
            { "Music/Library/Artist/Album/01 - Song.flac", @"Music\Library\Artist/Album/01 - Song.flac" },
            { @"F:\Music/Library/Artist/Album/01 - Song.flac", @"F:\Music\Library\Artist/Album/01 - Song.flac" },
            { @"F:/Music/Library/Artist/Album/01 - Song.flac", @"F:/Music\Library\Artist/Album/01 - Song.flac" },
            { @"/Library/Artist\Album\01 - Song.flac", @"\Library\Artist\Album\01 - Song.flac" },
            { @"/Library\Artist\Album\01 - Song.flac", @"/Library\Artist\Album\01 - Song.flac" },
            { @"\Library/Artist\Album\01 - Song.flac", @"\Library/Artist\Album\01 - Song.flac" },
        };
    }
}