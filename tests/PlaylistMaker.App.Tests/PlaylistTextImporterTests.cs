using PlaylistMaker.Application;
using PlaylistMaker.Core;

namespace PlaylistMaker.App.Tests;

public class PlaylistTextImporterTests
{
    [Fact]
    public void ImportsResolvableEntriesAndReportsAmbiguousAndMissingLines()
    {
        var map = new Dictionary<string, string>
        {
            [@"C:\one\240101 Artist - One.mkv"] = @"C:\audio\one.flac",
            [@"C:\two\240101 same.mkv"] = @"C:\audio\two.flac",
            [@"C:\three\240101 same.mkv"] = @"C:\audio\three.flac",
        };
        var catalog = new MediaLibraryCatalog(new MediaLibraryCatalogTests.StubVorbisReader(map.Values), map);
        var result = new PlaylistTextImporter(catalog).ImportLines([
            "240101 Artist - One.mkv",
            "240101 same.mkv",
            "missing.mkv",
        ]);

        Assert.Single(result.Videos);
        Assert.Equal(2, result.Issues.Count);
        Assert.Contains(result.Issues, issue => issue.Reason.Contains("ambiguous"));
        Assert.Contains(result.Issues, issue => issue.Reason.Contains("No matching"));
    }
}
