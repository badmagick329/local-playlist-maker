using System.Globalization;
using System.Text.Json;
using System.Text.Json.Nodes;
using PlaylistMaker.Application;
using PlaylistMaker.Core;
using PlaylistMaker.Infrastructure;

namespace PlaylistMaker.App.Tests;

public sealed class CharmBackendFixtureTests : IDisposable
{
    private readonly string _root = Path.Combine(Path.GetTempPath(), "playlistmaker-charm-fixture-", Guid.NewGuid().ToString("N"));

    [Fact]
    public void LibraryBasicFixtureProducesTheCanonicalSnapshot()
    {
        using var fixture = CopyFixture("library-basic");
        var config = new ConfigReader(Path.Combine(fixture.Directory, "config.yaml")).ReadConfig();
        var mappingFiles = config.MusicVideoToAudioMap
            .Select(path => Path.Combine(fixture.Directory, path))
            .ToList();
        var map = new ImportedVideoToAudioMap(mappingFiles).Import();
        var reader = new VorbisReader(
            new FlacPathsReader(
                Path.Combine(fixture.Directory, config.FlacsMegaPlaylist),
                mappingFiles[0]
            ),
            Path.Combine(fixture.Directory, config.DataDirectory, config.FlacCacheFile)
        );
        var catalog = new MediaLibraryCatalog(reader, map);
        var snapshot = BackendSnapshotFactory.Create(catalog, new PlaybackHistoryIndex());

        var actual = JsonSerializer.SerializeToNode(Canonical(snapshot, fixture.Directory));
        var expected = JsonNode.Parse(File.ReadAllText(Path.Combine(
            Directory.GetParent(fixture.SourceDirectory)!.FullName,
            "library-basic.expected.json"
        )));

        Assert.True(
            JsonNode.DeepEquals(expected, actual),
            $"Fixture snapshot differs.\nExpected: {expected}\nActual: {actual}"
        );

        var defaultVariant = MediaLibraryCatalog.SelectDefault(
            Assert.Single(catalog.Tracks, track => track.Track.Artist == "AURORA").Variants
        );
        Assert.Equal("240101 AURORA - Northern Lights.mkv", defaultVariant.FileName);
    }

    private Fixture CopyFixture(string name)
    {
        var source = Path.Combine(AppContext.BaseDirectory, "testdata", "charm-backend", name);
        var destination = Path.Combine(_root, name);
        foreach (var directory in Directory.GetDirectories(source, "*", SearchOption.AllDirectories))
        {
            Directory.CreateDirectory(directory.Replace(source, destination, StringComparison.Ordinal));
        }
        foreach (var file in Directory.GetFiles(source, "*", SearchOption.AllDirectories))
        {
            var target = file.Replace(source, destination, StringComparison.Ordinal);
            Directory.CreateDirectory(Path.GetDirectoryName(target)!);
            File.Copy(file, target);
            if (Path.GetExtension(file) is ".json" or ".yaml" or ".m3u8")
            {
                var replacement = Path.GetExtension(file) == ".json"
                    ? destination.Replace("\\", "\\\\")
                    : destination;
                File.WriteAllText(target, File.ReadAllText(target).Replace("@ROOT@", replacement));
            }
        }

        var manifest = JsonNode.Parse(File.ReadAllText(Path.Combine(destination, "manifest.json")))!;
        foreach (var (relativePath, timestamp) in manifest["videoModificationTimesUtc"]!.AsObject())
        {
            var path = Path.Combine(destination, relativePath.Replace('/', Path.DirectorySeparatorChar));
            Directory.CreateDirectory(Path.GetDirectoryName(path)!);
            File.WriteAllText(path, string.Empty);
            File.SetLastWriteTimeUtc(path, timestamp!.GetValue<DateTime>());
        }
        return new Fixture(source, destination);
    }

    private static object Canonical(BackendLibrarySnapshot snapshot, string root) => new
    {
        schemaVersion = snapshot.SchemaVersion,
        tracks = snapshot.Tracks.Select(track => new
        {
            id = Portable(track.Id, root),
            artist = track.Artist,
            title = track.Title,
            releaseDate = track.ReleaseDate,
            history = Canonical(track.History),
            variants = track.Variants.Select(variant => new
            {
                id = Portable(variant.Id, root),
                category = variant.Category,
                videoDate = variant.VideoDate,
                modifiedAtUtc = variant.ModifiedAtUtc.ToString("O", CultureInfo.InvariantCulture),
                history = Canonical(variant.History),
            }),
        }),
    };

    private static object Canonical(BackendHistory history) => new
    {
        playedCount = history.PlayedCount,
        completedCount = history.CompletedCount,
        stoppedCount = history.StoppedCount,
        skippedCount = history.SkippedCount,
        lastPlayedAtUtc = history.LastPlayedAtUtc?.ToString("O", CultureInfo.InvariantCulture),
    };

    private static string Portable(string path, string root) => path
        .Replace(root, "@ROOT@", StringComparison.OrdinalIgnoreCase)
        .Replace('\\', '/');

    public void Dispose()
    {
        if (Directory.Exists(_root)) Directory.Delete(_root, true);
    }

    private sealed record Fixture(string SourceDirectory, string Directory) : IDisposable
    {
        public void Dispose() { }
    }
}
