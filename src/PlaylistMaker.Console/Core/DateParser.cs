using System.Text.RegularExpressions;

namespace PlaylistMaker.Core;

internal static partial class DateParser
{
    public static ReleaseDate? TryParseReleaseDate(string date)
    {
        if (string.IsNullOrWhiteSpace(date))
        {
            return null;
        }

        var fullDate = TryParseFullDate(date);
        if (fullDate is not null)
        {
            return new ReleaseDate(fullDate.Value.year, fullDate.Value.month,
                fullDate.Value.day);
        }

        var yearMonthDate = TryParseYearMonthDate(date);
        if (yearMonthDate is not null)
        {
            return new ReleaseDate(yearMonthDate.Value.year,
                yearMonthDate.Value.month);
        }

        var yearDate = TryParseYearDate(date);
        if (yearDate is not null)
        {
            return new ReleaseDate(yearDate.Value);
        }

        return null;
    }

    private static (int year, int month, int day)? TryParseFullDate(string date)
    {
        var match = FullDateRegex().Match(date);
        if (!match.Success)
        {
            return null;
        }

        (string year, string month, string day) = (
            match.Groups["year"].Value,
            match.Groups["month"].Value,
            match.Groups["day"].Value
        );
        return TryParseFullDate(year, month, day);
    }

    [GeneratedRegex(@"^(?<year>\d{4})[-/.](?<month>\d{2})[-/.](?<day>\d{2})$")]
    private static partial Regex FullDateRegex();

    private static (int year, int month, int day)? TryParseFullDate(
        string year,
        string month,
        string day
    )
    {
        var yearInt = int.Parse(year);
        var monthInt = int.Parse(month);
        var dayInt = int.Parse(day);

        try
        {
            _ = new DateOnly(yearInt, monthInt, dayInt);
        }
        catch
        {
            return null;
        }

        return (yearInt, monthInt, dayInt);
    }

    private static (int year, int month)? TryParseYearMonthDate(string date)
    {
        var match = YearMonthDateRegex().Match(date);

        if (!match.Success)
            return null;

        var year = int.Parse(match.Groups["year"].Value);
        var month = int.Parse(match.Groups["month"].Value);
        if (!IsValidYear(year) || !IsValidMonth(month))
            return null;

        return (year, month);
    }

    [GeneratedRegex(@"^(?<year>\d{4})[-/.](?<month>\d{2})$")]
    private static partial Regex YearMonthDateRegex();

    private static int? TryParseYearDate(string date)
    {
        var match = YearDateRegex().Match(date);

        if (!match.Success)
            return null;

        var year = int.Parse(match.Groups["year"].Value);
        return !IsValidYear(year) ? null : year;
    }

    [GeneratedRegex(@"^(?<year>\d{4})$")]
    private static partial Regex YearDateRegex();

    public static List<ReleaseDate> FindFullDates(string text)
    {
        var matches = FullYearDateStringRegex().Matches(text).Select(m => m);
        matches = matches.Concat(TrimmedYearDateStringRegex().Matches(text)
            .Select(m => m));

        var releaseDates = new List<ReleaseDate>();
        foreach (Match match in matches)
        {
            var year = match.Groups["year"].Value;
            if (year.Length == 2)
            {
                year = $"20{year}";
            }

            var month = match.Groups["month"].Value;
            var day = match.Groups["day"].Value;
            var releaseDate = TryParseFullDate(year, month, day);
            if (releaseDate is not null)
            {
                releaseDates.Add(
                    new ReleaseDate(
                        releaseDate.Value.year,
                        releaseDate.Value.month,
                        releaseDate.Value.day
                    )
                );
            }
        }

        return releaseDates;
    }

    [GeneratedRegex(@"\b(?<year>\d\d)(?<month>[01]\d)(?<day>[0123]\d)\b")]
    private static partial Regex TrimmedYearDateStringRegex();

    [GeneratedRegex(
        @"\b(?<year>[12]\d{3})[-./]?(?<month>[01]\d)[-./]?(?<day>[0123]\d)\b")]
    private static partial Regex FullYearDateStringRegex();

    private static bool IsValidYear(int year) => year is >= 1900 and <= 2100;

    private static bool IsValidMonth(int month) => month is >= 1 and <= 12;

    public static bool IsValidDate(int year, int month, int day)
    {
        try
        {
            _ = new DateOnly(year, month, day);
            return true;
        }
        catch
        {
            return false;
        }
    }
}
