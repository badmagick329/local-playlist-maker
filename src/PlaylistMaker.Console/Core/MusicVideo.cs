namespace PlaylistMaker.Core;

public class MusicVideo
{
    public string FilePath { get; init; }
    public Track Track { get; init; }
    public string Artist => Track.Artist;
    public string Title => Track.Title;
    public DateOnly FullDate => Track.FullDate;
    public ReleaseDate Date => Track.Date;
    public ReleaseDate VideoDate { get; init; }

    public MusicVideo(string filePath, Track track)
    {
        FilePath = filePath;
        Track = track;
        VideoDate = ReleaseDateFactory.FromVideoPath(filePath);
    }

    public override int GetHashCode() => HashCode.Combine(FilePath);
    public override bool Equals(object? obj) => obj is MusicVideo other && FilePath == other.FilePath;
}
