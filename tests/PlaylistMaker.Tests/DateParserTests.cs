using PlaylistMaker.Core;

namespace PlaylistMaker.Tests;

public class DateParserTests
{
    [Theory]
    [MemberData(nameof(FindFullDatesReturningReleaseDates))]
    public void FindFullDates_DateIsFound_WhenDateIsPresent(string text,
        List<ReleaseDate> expected)
    {
        var result = DateParser.FindFullDates(text);
        Assert.True(expected.Count == result.Count);

        expected.ForEach(e => Assert.Contains(e, result));
    }

    [Theory]
    [MemberData(nameof(TryParseReleaseDateReturningFullDate))]
    public void TryParseReleaseDate_FullDateIsFound_WhenFullDateIsPresent(
        string text,
        ReleaseDate expected
    )
    {
        var result = DateParser.TryParseReleaseDate(text);
        Assert.Equal(expected, result);
    }

    [Theory]
    [MemberData(nameof(TryParseReleaseDateReturningYearMonthDate))]
    public void TryParseReleaseDate_YearMonthDateIsFound_WhenYearMonthDateIsPresent(
        string text,
        ReleaseDate expected
    )
    {
        var result = DateParser.TryParseReleaseDate(text);
        Assert.Equal(expected, result);
    }

    [Theory]
    [MemberData(nameof(TryParseReleaseDateReturningYearDate))]
    public void TryParseReleaseDate_YearDateIsFound_WhenYearDateIsPresent(
        string text,
        ReleaseDate expected
    )
    {
        var result = DateParser.TryParseReleaseDate(text);
        Assert.Equal(expected, result);
    }

    public static TheoryData<string, List<ReleaseDate>>
        FindFullDatesReturningReleaseDates()
    {
        return new TheoryData<string, List<ReleaseDate>>
        {
            { "2021-10-14", [new ReleaseDate(2021, 10, 14)] },
            { "20211014", [new ReleaseDate(2021, 10, 14)] },
            { "211014", [new ReleaseDate(2021, 10, 14)] },
            { "2021/10-14", [new ReleaseDate(2021, 10, 14)] },
            { "2021.10/14", [new ReleaseDate(2021, 10, 14)] },
            {
                @"F:\Music\Library\NMIXX\230320 expergo\03. PAXXWORD.flac",
                [new ReleaseDate(2023, 3, 20)]
            },
            {
                @"F:\Music\Library\NMIXX\230320 expergo\03. PAXXWORD.flac",
                [new ReleaseDate(2023, 3, 20)]
            },
            {
                @"F:\Music\Library\T-ara\[2010.02.22] Breaking Heart (Repackage)\01 - Crazy Because of You.flac",
                [new ReleaseDate(2010, 02, 22)]
            },
            {
                @"F:\Music\Library\Everglow\200921 -77.82X-78.29\01 LA DI DA.flac",
                [new ReleaseDate(2020, 9, 21)]
            },
            {
                @"F:\Music\Library\IU\IU - 2013 - Modern Times - Epilogue／24bit／96khz／1.14GB\06 The Red Shoes.flac",
                []
            },
            {
                @"F:\Music\Library\Whee in\210413 Whee In (휘인) - Redd [2021.04.13] [EP] [WEB-FLAC]\01 Whee In - water color.flac",
                [new ReleaseDate(2021, 4, 13), new ReleaseDate(2021, 4, 13)]
            },
            {
                @"F:\Music\Library\Whee in\210413 Whee In (휘인) - Redd 2022.04.13 [EP] [WEB-FLAC]\01 Whee In - water color.flac",
                [new ReleaseDate(2022, 4, 13), new ReleaseDate(2021, 4, 13)]
            },
            {
                @"F:\Music\Library\Whee in\210413 Whee In (휘인) - Redd 2022.04.13 [EP] [WEB-FLAC]\01 Whee In - water color.flac",
                [new ReleaseDate(2022, 4, 13), new ReleaseDate(2021, 4, 13)]
            },
            {
                @"F:\Music\Library\000101 휘인210101",
                [new ReleaseDate(2000, 1, 1)]
            },
            { @"F:\Music\Library\00010 휘인210101", [] },
        };
    }

    // public static TheoryData<string,

    public static TheoryData<string, ReleaseDate>
        TryParseReleaseDateReturningFullDate()
    {
        return new TheoryData<string, ReleaseDate>
        {
            { "2021-10-14", new ReleaseDate(2021, 10, 14) },
            { "2021/10-14", new ReleaseDate(2021, 10, 14) },
            { "2021.10/14", new ReleaseDate(2021, 10, 14) },
            { "2000-01-30", new ReleaseDate(2000, 01, 30) },
            { "1900-01-30", new ReleaseDate(1900, 01, 30) },
            { "2100-01-30", new ReleaseDate(2100, 01, 30) },
        };
    }

    public static TheoryData<string, ReleaseDate>
        TryParseReleaseDateReturningYearMonthDate()
    {
        return new TheoryData<string, ReleaseDate>
        {
            { "2021-10", new ReleaseDate(2021, 10) },
            { "2021/10", new ReleaseDate(2021, 10) },
            { "2021.10", new ReleaseDate(2021, 10) },
            { "2000-01", new ReleaseDate(2000, 01) },
            { "1900-01", new ReleaseDate(1900, 01) },
            { "2100-01", new ReleaseDate(2100, 01) },
        };
    }

    public static TheoryData<string, ReleaseDate>
        TryParseReleaseDateReturningYearDate()
    {
        return new TheoryData<string, ReleaseDate>
        {
            { "2021", new ReleaseDate(2021) },
            { "2000", new ReleaseDate(2000) },
            { "1900", new ReleaseDate(1900) },
            { "2100", new ReleaseDate(2100) },
        };
    }
}