namespace PlaylistMaker.Core;

public class ReleaseDate
{
    public string AsString { get; init; }
    public int Year { get; init; }
    public int? Month { get; init; }
    public int? Day { get; init; }
    public DateType Type { get; init; }

    public ReleaseDate(int year, int? month = null, int? day = null)
    {
        Year = year;
        Month = month;
        Day = day;
        AsString = $"{Year}";
        if (Month is not null)
        {
            AsString += $"-{Month:D2}";
            if (Day is not null)
            {
                AsString += $"-{Day:D2}";
            }
        }

        Type = (Month, Day) switch
        {
            (not null, not null) => DateType.FullDate,
            (not null, null) => DateType.YearMonthDate,
            (null, _) => DateType.YearDate
        };
    }

    public bool IsAfterDateInclusive(int year, int? month = null,
        int? day = null) =>
        (month, day) switch
        {
            (int m, int d) => Year > year ||
                              (Year == year && (Month is null || Month > m ||
                                                (Month == m &&
                                                 (Day is null || Day >= d)))),

            (int m, null) => Year > year ||
                             (Year == year && (Month is null || Month >= m)),

            (null, _) => Year >= year,
        };

    public bool IsBeforeDateInclusive(int year, int? month = null,
        int? day = null) =>
        (month, day) switch
        {
            (int m, int d) => Year < year
                              || (
                                  Year == year
                                  && (Month is null || Month < m ||
                                      (Month == m && (Day is null || Day <= d)))
                              ),

            (int m, null) => Year < year ||
                             (Year == year && (Month is null || Month <= m)),

            (null, _) => Year <= year,
        };

    public bool IsAtDate(int year, int? month = null, int? day = null) =>
        Year == year && (Month is null || Month == month) &&
        (Day is null || Day == day);


    public override string ToString() => AsString;

    public override bool Equals(object? obj)
    {
        if (obj is not ReleaseDate other)
        {
            return false;
        }

        return Year == other.Year && Month == other.Month && Day == other.Day;
    }

    public override int GetHashCode() => HashCode.Combine(Year, Month, Day);
}

public enum DateType
{
    FullDate,
    YearMonthDate,
    YearDate
}
