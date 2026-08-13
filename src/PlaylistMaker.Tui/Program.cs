using PlaylistMaker.Application;
using PlaylistMaker.Core;
using PlaylistMaker.Infrastructure;
using Terminal.Gui.App;

namespace PlaylistMaker.Tui;

internal static class Program
{
    private const string ConfigPath = "config.yaml";

    public static int Main(string[] args)
    {
        if (!File.Exists(ConfigPath))
        {
            Console.Error.WriteLine($"{ConfigPath} not found");
            return 1;
        }

        try
        {
            var config = new ConfigReader(ConfigPath).ReadConfig();
            Directory.CreateDirectory(config.DataDirectory);
            var flacPathsReader = new FlacPathsReader(config.FlacsMegaPlaylist, config.MusicVideoToAudioMap[0]);
            var vorbisReader = new VorbisReader(
                flacPathsReader,
                Path.Combine(config.DataDirectory, config.FlacCacheFile)
            );
            MediaLibraryCatalog LoadCatalog() => new(
                vorbisReader,
                new ImportedVideoToAudioMap(config.MusicVideoToAudioMap).Import()
            );
            var catalog = LoadCatalog();

            if (args.Contains("--check", StringComparer.OrdinalIgnoreCase))
            {
                var historyIndex = new PlaybackHistoryReader(
                    Path.Combine(config.DataDirectory, "play-history.jsonl")
                ).Read();
                Console.WriteLine($"Videos: {catalog.Videos.Count}");
                Console.WriteLine($"Tracks: {catalog.Tracks.Count}");
                Console.WriteLine($"Official-MV tracks visible by default: {catalog.Search(new LibraryQuery()).Count}");
                Console.WriteLine($"Invalid history lines: {historyIndex.InvalidLineCount}");
                return 0;
            }

            PlaybackHistoryService? historyService = null;
            if (config.PlaybackHistoryEnabled)
            {
                historyService = new PlaybackHistoryService(
                    config.DataDirectory,
                    config.PlaybackHistoryMinimumWatchedPercent
                );
                historyService.RecoverIncompleteSessions();
            }

            var historyReader = new PlaybackHistoryReader(
                Path.Combine(config.DataDirectory, "play-history.jsonl")
            );
            var state = new TuiState(catalog, historyReader);
            var services = new TuiServices(
                config,
                state,
                new QueuePlaybackService(
                    new PlaybackPlanner(),
                    new PlaybackCoordinator(
                        PlaylistPlayerFactory.CreateAudio(config),
                        PlaylistPlayerFactory.CreateVideo(config),
                        historyService
                    )
                ),
                LoadCatalog,
                new MpvScriptManager()
            );

            using var app = Terminal.Gui.App.Application.Create().Init();
            using var window = new MainWindow(services);
            app.Run(window);
            return 0;
        }
        catch (Exception exception)
        {
            Console.Error.WriteLine($"PlaylistMaker TUI could not start:{Environment.NewLine}{exception}");
            return 1;
        }
    }
}

public sealed record TuiServices(
    Config Config,
    TuiState State,
    QueuePlaybackService Playback,
    Func<MediaLibraryCatalog> LoadCatalog,
    MpvScriptManager ScriptManager
);
