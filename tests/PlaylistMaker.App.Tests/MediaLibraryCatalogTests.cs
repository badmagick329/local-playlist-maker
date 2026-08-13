using PlaylistMaker.Application;
using PlaylistMaker.Core;

namespace PlaylistMaker.App.Tests;

public class MediaLibraryCatalogTests
{
    [Fact]
    public void GroupsVideosByMappedAudioPathAndKeepsDuplicateFileNames()
    {
        var catalog = CreateCatalog(new Dictionary<string, string>
        {
            [@"C:\videos\a\240101 Artist - Song.mkv"] = @"C:\audio\song.flac",
            [@"C:\videos\b\240102 Artist - Song.mkv"] = @"C:\audio\song.flac",
            [@"C:\videos\c\240101 Artist - Song.mkv"] = @"C:\audio\other.flac",
        });

        Assert.Equal(2, catalog.Tracks.Count);
        Assert.Equal(3, catalog.Videos.Count);
        Assert.Equal(2, catalog.FindByFileName("240101 Artist - Song.mkv").Count);
    }

    [Fact]
    public void ConsolidatesEquivalentWindowsPathsWithLastMappingWinning()
    {
        var map = new Dictionary<string, string>
        {
            [@"C:\videos\240101 Artist - Song.mkv"] = @"C:\audio\first.flac",
            [@"c:\VIDEOS\240101 Artist - Song.mkv"] = @"C:\audio\second.flac",
        };

        var catalog = CreateCatalog(map);

        Assert.Single(catalog.Videos);
        Assert.Equal(@"C:\audio\second.flac", Assert.Single(catalog.Videos).AudioPath);
    }

    [Fact]
    public void DefaultVariantPrefersOfficialMusicVideoThenNewest()
    {
        var catalog = CreateCatalog(new Dictionary<string, string>
        {
            [@"C:\videos\250101 Artist - Song Performance.mkv"] = @"C:\audio\song.flac",
            [@"C:\videos\230101 Artist - Song.mkv"] = @"C:\audio\song.flac",
            [@"C:\videos\240101 Artist - Song.mkv"] = @"C:\audio\song.flac",
        });
        var result = Assert.Single(catalog.Search(new LibraryQuery
        {
            Categories = Enum.GetValues<VideoCategory>().ToHashSet(),
        }));

        Assert.Equal("240101 Artist - Song.mkv", result.DefaultVariant.FileName);
    }

    [Fact]
    public void FiltersBeforeFuzzySearchingArtistTitleAndFilename()
    {
        var catalog = CreateCatalog(new Dictionary<string, string>
        {
            [@"C:\videos\240101 Billlie - WORK.mkv"] = @"C:\audio\work.flac",
            [@"C:\videos\240102 aespa - Supernova Performance.mkv"] = @"C:\audio\supernova.flac",
        });

        var official = catalog.Search(new LibraryQuery { SearchText = "Billie work" });
        var hiddenPerformance = catalog.Search(new LibraryQuery { SearchText = "Supernova" });
        var visiblePerformance = catalog.Search(new LibraryQuery
        {
            SearchText = "Supernova",
            Categories = new HashSet<VideoCategory> { VideoCategory.Performance },
        });

        Assert.Single(official);
        Assert.Empty(hiddenPerformance);
        Assert.Single(visiblePerformance);
    }

    private static MediaLibraryCatalog CreateCatalog(IReadOnlyDictionary<string, string> map) =>
        new(new StubVorbisReader(map.Values.Distinct()), map);

    internal sealed class StubVorbisReader(IEnumerable<string> paths) : IVorbisReader
    {
        private readonly Dictionary<string, VorbisData> _data = paths.ToDictionary(
            path => path,
            path =>
            {
                var name = Path.GetFileNameWithoutExtension(path);
                return new VorbisData(path, name.Contains("supernova") ? "aespa" : "Artist", name, "2024-01-01", 1, "now");
            },
            PathIdentity.Comparer
        );

        public VorbisData? VorbisDataFor(string filePath) => _data.GetValueOrDefault(filePath);
        public List<string> GetAllFilePaths() => _data.Keys.ToList();
    }
}
