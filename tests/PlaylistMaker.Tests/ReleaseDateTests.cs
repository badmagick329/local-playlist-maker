using PlaylistMaker.Core;

namespace PlaylistMaker.Tests;

public class ReleaseDateTests
{
    [Theory]
    [MemberData(nameof(ReleaseDateAsStringData))]
    public void ReleaseDateAsString_DateIsFound_WhenDateIsPresent(
        (int, int?, int?) date, string expected)
    {
        var result = new ReleaseDate(date.Item1, date.Item2, date.Item3);
        Assert.Equal(expected, result.AsString);
    }

    [Theory]
    [MemberData(nameof(IsAfterDateInclusiveData))]
    public void IsAfterDateInclusive_DateIsFound_WhenDateIsPresent(ReleaseDate date,
        (int, int?, int?) otherDate,
        bool expected)
    {
        var result = date.IsAfterDateInclusive(otherDate.Item1, otherDate.Item2,
            otherDate.Item3);
        Assert.Equal(expected, result);
    }

    [Theory]
    [MemberData(nameof(IsBeforeDateInclusiveData))]
    public void IsBeforeDateInclusive_DateIsFound_WhenDateIsPresent(
        ReleaseDate date, (int, int?, int?) otherDate,
        bool expected)
    {
        var result = date.IsBeforeDateInclusive(otherDate.Item1,
            otherDate.Item2, otherDate.Item3);
        Assert.Equal(expected, result);
    }

    [Theory]
    [MemberData(nameof(IsAtDateData))]
    public void IsAtDate_DateIsFound_WhenDateIsPresent(ReleaseDate date,
        (int, int?, int?) otherDate,
        bool expected)
    {
        var result = date.IsAtDate(otherDate.Item1, otherDate.Item2,
            otherDate.Item3);
        Assert.Equal(expected, result);
    }

    [Theory]
    [MemberData(nameof(ReleaseDateTypeData))]
    public void ReleaseDateType_DateIsFound_WhenDateIsPresent(
        (int, int?, int?) date, DateType expected)
    {
        var result = new ReleaseDate(date.Item1, date.Item2, date.Item3);
        Assert.Equal(expected, result.Type);
    }

    public static TheoryData<(int, int?, int?), string>
        ReleaseDateAsStringData()
    {
        return new TheoryData<(int, int?, int?), string>
        {
            { (2020, null, null), "2020" },
            { (2020, 1, null), "2020-01" },
            { (2020, 1, 1), "2020-01-01" },
            { (2020, null, 1), "2020" },
        };
    }

    public static TheoryData<ReleaseDate, (int, int?, int?), bool>
        IsAfterDateInclusiveData()
    {
        return new TheoryData<ReleaseDate, (int, int?, int?), bool>
        {
            { new ReleaseDate(2020, 1, 1), (2020, 1, 1), true },
            { new ReleaseDate(2020, 1, 1), (2020, 1, 2), false },
            { new ReleaseDate(2020, 1, 1), (2020, 2, 1), false },
            { new ReleaseDate(2020, 1, 1), (2021, 1, 1), false },
            { new ReleaseDate(2020, 1, 1), (2019, 1, 1), true },
            { new ReleaseDate(2020, 1, 1), (2019, 12, 31), true },
            { new ReleaseDate(2020), (2019, 12, 31), true },
            { new ReleaseDate(2020), (2020, 12, 31), true },
            { new ReleaseDate(2020), (2021, 1, 1), false },
            { new ReleaseDate(2020), (2021, null, null), false },
            { new ReleaseDate(2020), (2020, null, null), true },
            { new ReleaseDate(2020), (2019, null, null), true },
            { new ReleaseDate(2020), (2021, 1, null), false },
            { new ReleaseDate(2020), (2020, 1, null), true },
            { new ReleaseDate(2020), (2019, 12, null), true },
        };
    }

    public static TheoryData<ReleaseDate, (int, int?, int?), bool>
        IsBeforeDateInclusiveData()
    {
        return new TheoryData<ReleaseDate, (int, int?, int?), bool>
        {
            { new ReleaseDate(2020, 1, 1), (2020, 1, 1), true },
            { new ReleaseDate(2020, 1, 1), (2020, 1, 2), true },
            { new ReleaseDate(2020, 1, 1), (2020, 2, 1), true },
            { new ReleaseDate(2020, 1, 1), (2021, 1, 1), true },
            { new ReleaseDate(2020, 1, 1), (2019, 1, 1), false },
            { new ReleaseDate(2020, 1, 1), (2019, 12, 31), false },
            { new ReleaseDate(2020), (2019, 12, 31), false },
            { new ReleaseDate(2020), (2020, 12, 31), true },
            { new ReleaseDate(2020), (2021, 1, 1), true },
            { new ReleaseDate(2020), (2021, null, null), true },
            { new ReleaseDate(2020), (2020, null, null), true },
            { new ReleaseDate(2020), (2019, null, null), false },
            { new ReleaseDate(2020), (2021, 1, null), true },
            { new ReleaseDate(2020), (2020, 1, null), true },
            { new ReleaseDate(2020), (2019, 12, null), false },
        };
    }

    public static TheoryData<ReleaseDate, (int, int?, int?), bool>
        IsAtDateData()
    {
        return new TheoryData<ReleaseDate, (int, int?, int?), bool>
        {
            { new ReleaseDate(2020, 1, 1), (2020, 1, 1), true },
            { new ReleaseDate(2020, 1, 1), (2020, 1, 2), false },
            { new ReleaseDate(2020, 1, 1), (2020, 2, 1), false },
            { new ReleaseDate(2020, 1, 1), (2021, 1, 1), false },
            { new ReleaseDate(2020, 1, 1), (2019, 1, 1), false },
            { new ReleaseDate(2020, 1, 1), (2019, 12, 31), false },
            { new ReleaseDate(2020), (2019, 12, 31), false },
            { new ReleaseDate(2020), (2020, 12, 31), true },
            { new ReleaseDate(2020), (2021, 1, 1), false },
            { new ReleaseDate(2020), (2021, null, null), false },
            { new ReleaseDate(2020), (2020, null, null), true },
            { new ReleaseDate(2020), (2019, null, null), false },
            { new ReleaseDate(2020), (2021, 1, null), false },
            { new ReleaseDate(2020), (2020, 1, null), true },
            { new ReleaseDate(2020), (2019, 12, null), false },
        };
    }

    public static TheoryData<(int, int?, int?), DateType> ReleaseDateTypeData()
    {
        return new TheoryData<(int, int?, int?), DateType>
        {
            { (2020, null, null), DateType.YearDate },
            { (2020, 1, null), DateType.YearMonthDate },
            { (2020, 1, 1), DateType.FullDate },
        };
    }
}