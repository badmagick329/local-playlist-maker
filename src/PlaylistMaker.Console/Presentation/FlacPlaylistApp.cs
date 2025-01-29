using PlaylistMaker.Application;
using PlaylistMaker.Core;

namespace PlaylistMaker.Presentation;

public class FlacPlaylistApp
{
    private readonly IVorbisReader _vorbisReader;
    private readonly IUserInputReader _userInputReader;
    private readonly IPlaylistPlayer _flacPlaylistPlayer;

    public FlacPlaylistApp(IVorbisReader reader, IUserInputReader userInputReader,
        IPlaylistPlayer flacPlaylistPlayer)
    {
        _vorbisReader = reader;
        _userInputReader = userInputReader;
        _flacPlaylistPlayer = flacPlaylistPlayer;
    }

    public void Run()
    {
        var tracks =
            _vorbisReader.GetAllFilePaths()
                .Select(_vorbisReader.VorbisDataFor)
                .OfType<VorbisData>()
                .Select(TrackFactory.FromVorbisData)
                .OfType<Track>()
                .ToList();
        EnsureNoPartialDates(tracks);

        Console.WriteLine($"Valid Date Formats YYYY-MM-DD, YYYY-MM, YYYY");
        var startDate = _userInputReader.AskDate($"Enter start date filter") ?? new ReleaseDate(1900);
        var endDate = _userInputReader.AskDate($"Enter end date filter") ?? new ReleaseDate(2100);
        var trackedFilteredByDate =
            tracks
                .Where(t =>
                    t.Date.IsAfterDateInclusive(startDate.Year, startDate.Month, startDate.Day) &&
                    t.Date.IsBeforeDateInclusive(endDate.Year, endDate.Month, endDate.Day))
                .ToList();
        var allArtists = trackedFilteredByDate.Select(t => t.Artist).Distinct().ToList();
        var artists = _userInputReader.AskStringsContainedIn("Choose artists to filter by", allArtists);
        artists = artists.Count == 0 ? allArtists : artists;

        var filteredTracks =
            trackedFilteredByDate
                .Where(t => artists.Contains(t.Artist, StringComparer.InvariantCultureIgnoreCase))
                .Distinct()
                .OrderBy(t => t.FullDate)
                .ThenBy(t => t.Artist)
                .ThenBy(t => t.TrackNumber)
                .ToList();

        _flacPlaylistPlayer.CreateAndPlay(filteredTracks.Select(t => t.FilePath).ToList());
    }

    private static void EnsureNoPartialDates(List<Track> tracks)
    {
        if (tracks.Any(t => t.Date.Type is not DateType.FullDate))
        {
            throw new ArgumentException("Partial date found");
        }
    }
}