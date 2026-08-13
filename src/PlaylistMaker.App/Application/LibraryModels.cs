namespace PlaylistMaker.Core;

public enum VideoCategory
{
    MusicVideo,
    BandLive,
    Performance,
    Choreography,
    Relay,
    BeOriginal,
    Fancam,
    Concert,
    MusicShow,
    Remix,
    LiveAudio,
}

public enum LibrarySort
{
    NameAscending,
    NameDescending,
    ArtistAscending,
    ArtistDescending,
    TrackReleaseAscending,
    TrackReleaseDescending,
    VideoDateAscending,
    VideoDateDescending,
    ModifiedAscending,
    ModifiedDescending,
}

public sealed record DateRange(ReleaseDate? Start, ReleaseDate? End)
{
    public bool Contains(ReleaseDate date)
    {
        if (Start is not null && !date.IsAfterDateInclusive(Start.Year, Start.Month, Start.Day))
        {
            return false;
        }

        return End is null || date.IsBeforeDateInclusive(End.Year, End.Month, End.Day);
    }
}

public sealed record VideoVariant(
    string Id,
    MusicVideo Video,
    VideoCategory Category,
    DateTime ModifiedAtUtc
)
{
    public string VideoPath => Video.FilePath;
    public string AudioPath => Video.Track.FilePath;
    public string FileName => Path.GetFileName(VideoPath);
    public string Artist => Video.Artist;
    public string Title => Video.Title;
    public ReleaseDate VideoDate => Video.VideoDate;
}

public sealed record TrackGroup(
    string Id,
    Track Track,
    IReadOnlyList<VideoVariant> Variants
);

public sealed class LibraryQuery
{
    public string SearchText { get; init; } = string.Empty;
    public IReadOnlySet<VideoCategory> Categories { get; init; } =
        new HashSet<VideoCategory> { VideoCategory.MusicVideo };
    public DateRange? TrackDate { get; init; }
    public DateRange? VideoDate { get; init; }
    public LibrarySort Sort { get; init; } = LibrarySort.NameDescending;
}

public sealed record TrackSearchResult(
    TrackGroup Group,
    IReadOnlyList<VideoVariant> EligibleVariants,
    VideoVariant DefaultVariant,
    int SearchScore
);

public static class PathIdentity
{
    public static StringComparer Comparer { get; } = OperatingSystem.IsWindows()
        ? StringComparer.OrdinalIgnoreCase
        : StringComparer.Ordinal;

    public static string Normalize(string path) => Path.GetFullPath(path)
        .TrimEnd(Path.DirectorySeparatorChar, Path.AltDirectorySeparatorChar);
}
