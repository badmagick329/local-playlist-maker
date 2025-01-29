using PlaylistMaker.Core;

namespace PlaylistMaker.Tests;

public class ReleaseDateFactoryTests
{
    [Theory]
    [MemberData(nameof(TrackDateInitializationData))]
    public void TrackDateInitialization_DateIsFound_WhenDateIsPresent(
        string date,
        string path,
        ReleaseDate expected
    )
    {
        var result = ReleaseDateFactory.TryFromTagOrPath(date, path);
        Assert.NotNull(result);
        Assert.Equal(expected, result);
    }

    public static TheoryData<string, string, ReleaseDate>
        TrackDateInitializationData()
    {
        return new TheoryData<string, string, ReleaseDate>
        {
            { "2021-10-14", "", new ReleaseDate(2021, 10, 14) },
            { "2021/10-14", "", new ReleaseDate(2021, 10, 14) },
            {
                "", @"F:\Music\Library\NMIXX\230320 expergo\03. PAXXWORD.flac",
                new ReleaseDate(2023, 3, 20)
            },
            {
                "2023-03-20",
                @"F:\Music\Library\NMIXX\230320 expergo\03. PAXXWORD.flac",
                new ReleaseDate(2023, 3, 20)
            },
            {
                "2014-11-11",
                @"F:\Music\Library\AoA\141111 Like a Cat\01 - Like a Cat.flac",
                new ReleaseDate(2014, 11, 11)
            },
            {
                "2014-11-11",
                @"F:\Music\Library\AoA\Like a Cat\01 - Like a Cat.flac",
                new ReleaseDate(2014, 11, 11)
            },
        };
    }
}