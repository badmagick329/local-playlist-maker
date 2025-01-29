namespace PlaylistMaker.Core;

public class Track
{
    public int TrackNumber { get; init; }
    public string Artist { get; init; }
    public string Title { get; init; }
    public ReleaseDate Date { get; init; }
    public string FilePath { get; init; }
    public DateOnly FullDate { get; init; }

    public Track(int trackNumber, string artist, string title, ReleaseDate date,
        string filePath)
    {
        TrackNumber = trackNumber;
        Artist = artist;
        Title = title;
        Date = date;
        FilePath = filePath;
        FullDate = date.Type switch
        {
            DateType.FullDate => new DateOnly(date.Year, date.Month!.Value,
                date.Day!.Value),
            DateType.YearMonthDate => new DateOnly(date.Year, date.Month!.Value,
                1),
            DateType.YearDate => new DateOnly(date.Year, 1, 1),
            _ => throw new ArgumentException($"Invalid date type {date.Type}")
        };
    }

    public override string ToString()
    {
        return $"[{TrackNumber}] {Artist} - {Title} ({Date}) -- ({FullDate})";
    }
}
