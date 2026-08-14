using System.Text.Json;
using PlaylistMaker.Application;
using PlaylistMaker.Core;
using PlaylistMaker.Infrastructure;

namespace PlaylistMaker.Bridge;

internal static class Program
{
    private static readonly JsonSerializerOptions JsonOptions = new()
    {
        PropertyNamingPolicy = JsonNamingPolicy.CamelCase,
        PropertyNameCaseInsensitive = true,
    };

    public static int Main(string[] args)
    {
        using var protocol = new StreamWriter(Console.OpenStandardOutput()) { AutoFlush = true };
        Console.SetOut(Console.Error);

        try
        {
            var configPath = ArgumentValue(args, "--config") ?? "config.yaml";
            var disableHistory = args.Contains("--disable-history", StringComparer.OrdinalIgnoreCase);
            var services = BridgeServices.Create(configPath, disableHistory);
            Write(protocol, new BridgeResponse(
                0,
                "ready",
                true,
                services.CreateSnapshot(),
                null
            ));

            while (Console.In.ReadLine() is { } line)
            {
                BridgeRequest? request;
                try
                {
                    request = JsonSerializer.Deserialize<BridgeRequest>(line, JsonOptions);
                }
                catch (JsonException exception)
                {
                    Write(protocol, new BridgeResponse(0, "error", false, null, exception.Message));
                    continue;
                }

                if (request is null)
                {
                    continue;
                }

                if (request.Type.Equals("shutdown", StringComparison.OrdinalIgnoreCase))
                {
                    Write(protocol, new BridgeResponse(request.Id, "shutdown", true, null, null));
                    return 0;
                }

                Handle(protocol, services, request);
            }

            return 0;
        }
        catch (Exception exception)
        {
            Write(protocol, new BridgeResponse(0, "ready", false, null, exception.ToString()));
            return 1;
        }
    }

    private static void Handle(StreamWriter protocol, BridgeServices services, BridgeRequest request)
    {
        try
        {
            switch (request.Type.ToLowerInvariant())
            {
                case "play":
                    var result = services.Play(request.VideoIds, request.Options ?? new());
                    Write(protocol, new BridgeResponse(request.Id, "play", result.Succeeded, result, result.Error));
                    break;
                default:
                    Write(protocol, new BridgeResponse(
                        request.Id,
                        request.Type,
                        false,
                        null,
                        $"Unknown bridge request: {request.Type}"
                    ));
                    break;
            }
        }
        catch (Exception exception)
        {
            Write(protocol, new BridgeResponse(request.Id, request.Type, false, null, exception.Message));
        }
    }

    private static string? ArgumentValue(string[] args, string name)
    {
        var index = Array.FindIndex(args, value => value.Equals(name, StringComparison.OrdinalIgnoreCase));
        return index >= 0 && index + 1 < args.Length ? args[index + 1] : null;
    }

    private static void Write(StreamWriter writer, BridgeResponse response) =>
        writer.WriteLine(JsonSerializer.Serialize(response, JsonOptions));
}

internal sealed class BridgeServices
{
    private readonly MediaLibraryCatalog _catalog;
    private readonly PlaybackHistoryIndex _history;
    private readonly QueuePlaybackService _playback;

    private BridgeServices(
        MediaLibraryCatalog catalog,
        PlaybackHistoryIndex history,
        QueuePlaybackService playback
    )
    {
        _catalog = catalog;
        _history = history;
        _playback = playback;
    }

    public static BridgeServices Create(string configPath, bool disableHistory = false)
    {
        if (!File.Exists(configPath))
        {
            throw new FileNotFoundException("PlaylistMaker config was not found.", configPath);
        }

        var config = new ConfigReader(configPath).ReadConfig();
        Directory.CreateDirectory(config.DataDirectory);
        var flacPathsReader = new FlacPathsReader(config.FlacsMegaPlaylist, config.MusicVideoToAudioMap[0]);
        var vorbisReader = new VorbisReader(
            flacPathsReader,
            Path.Combine(config.DataDirectory, config.FlacCacheFile)
        );
        var catalog = new MediaLibraryCatalog(
            vorbisReader,
            new ImportedVideoToAudioMap(config.MusicVideoToAudioMap).Import()
        );

        PlaybackHistoryService? historyService = null;
        if (config.PlaybackHistoryEnabled && !disableHistory)
        {
            historyService = new PlaybackHistoryService(
                config.DataDirectory,
                config.PlaybackHistoryMinimumWatchedPercent
            );
            historyService.RecoverIncompleteSessions();
        }

        var history = new PlaybackHistoryReader(
            Path.Combine(config.DataDirectory, "play-history.jsonl")
        ).Read();
        var playback = new QueuePlaybackService(
            new PlaybackPlanner(),
            new PlaybackCoordinator(
                PlaylistPlayerFactory.CreateAudio(config, isolateChildProcessIO: true),
                PlaylistPlayerFactory.CreateVideo(config, isolateChildProcessIO: true),
                historyService
            )
        );
        return new BridgeServices(catalog, history, playback);
    }

    public BridgeSnapshot CreateSnapshot() => new(
        1,
        _catalog.Tracks.Select(track => new BridgeTrack(
            track.Id,
            track.Track.Artist,
            track.Track.Title,
            track.Track.Date.AsString,
            ToHistory(_history.ForTrack(track.Track.FilePath)),
            track.Variants.Select(variant => new BridgeVariant(
                variant.Id,
                variant.VideoPath,
                variant.AudioPath,
                variant.FileName,
                CategoryLabel(variant.Category),
                variant.VideoDate.AsString,
                variant.ModifiedAtUtc,
                ToHistory(_history.ForVideo(variant.VideoPath))
            )).ToList()
        )).ToList()
    );

    public BridgePlayResult Play(IReadOnlyList<string> videoIds, BridgePlaybackOptions options)
    {
        var missing = new List<string>();
        var draft = new PlaylistDraft();
        foreach (var videoId in videoIds)
        {
            var variant = _catalog.FindByPath(videoId);
            if (variant is null)
            {
                missing.Add(videoId);
            }
            else
            {
                draft.Add(variant);
            }
        }

        if (missing.Count > 0)
        {
            return new BridgePlayResult(false, 0, null, $"{missing.Count} queued videos no longer exist in the library.");
        }

        var result = _playback.Launch(
            draft,
            new PlaybackOptions(
                options.Shuffle,
                options.MaximumItems,
                options.RepeatEach,
                options.OneVideoPerTrack
            ),
            "charm-tui"
        );
        return new BridgePlayResult(
            result.Launch.Succeeded,
            result.PlannedVideoCount,
            result.Launch.MpvProcessId,
            result.Launch.Error
        );
    }

    private static BridgeHistory ToHistory(PlaybackHistorySummary history) => new(
        history.PlayedCount,
        history.CompletedCount,
        history.StoppedCount,
        history.SkippedCount,
        history.LastPlayedAtUtc
    );

    private static string CategoryLabel(VideoCategory category) => category switch
    {
        VideoCategory.MusicVideo => "Music Video",
        VideoCategory.BandLive => "Band Live",
        VideoCategory.BeOriginal => "Be Original",
        VideoCategory.LiveAudio => "Live Audio",
        VideoCategory.MusicShow => "Music Show",
        _ => category.ToString(),
    };
}

internal sealed record BridgeRequest(
    int Id,
    string Type,
    IReadOnlyList<string> VideoIds,
    BridgePlaybackOptions? Options
);

internal sealed record BridgePlaybackOptions(
    bool Shuffle = false,
    int MaximumItems = 0,
    int RepeatEach = 1,
    bool OneVideoPerTrack = false
);

internal sealed record BridgeResponse(int Id, string Type, bool Ok, object? Result, string? Error);
internal sealed record BridgeSnapshot(int SchemaVersion, IReadOnlyList<BridgeTrack> Tracks);
internal sealed record BridgeTrack(
    string Id,
    string Artist,
    string Title,
    string ReleaseDate,
    BridgeHistory History,
    IReadOnlyList<BridgeVariant> Variants
);
internal sealed record BridgeVariant(
    string Id,
    string VideoPath,
    string AudioPath,
    string FileName,
    string Category,
    string VideoDate,
    DateTime ModifiedAtUtc,
    BridgeHistory History
);
internal sealed record BridgeHistory(
    int PlayedCount,
    int CompletedCount,
    int StoppedCount,
    int SkippedCount,
    DateTime? LastPlayedAtUtc
);
internal sealed record BridgePlayResult(
    bool Succeeded,
    int PlannedVideoCount,
    int? MpvProcessId,
    string? Error
);
