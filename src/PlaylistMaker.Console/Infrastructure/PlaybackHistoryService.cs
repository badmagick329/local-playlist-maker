using System.Diagnostics;
using System.Text;
using System.Text.Json;
using PlaylistMaker.Core;

namespace PlaylistMaker.Infrastructure;

public class PlaybackHistoryService
{
    private const string SessionsDirectoryName = "playback-sessions";
    private const string HistoryFileName = "play-history.jsonl";
    private static readonly JsonSerializerOptions JsonOptions = new()
    {
        PropertyNamingPolicy = JsonNamingPolicy.CamelCase,
        PropertyNameCaseInsensitive = true,
    };

    private readonly string _sessionsDirectory;
    private readonly string _historyPath;
    private readonly int _minimumWatchedPercent;

    public PlaybackHistoryService(string dataDirectory, int minimumWatchedPercent)
    {
        if (minimumWatchedPercent is < 1 or > 100)
        {
            throw new ArgumentOutOfRangeException(
                nameof(minimumWatchedPercent),
                "Playback history minimum watched percent must be between 1 and 100"
            );
        }

        _sessionsDirectory = Path.Combine(dataDirectory, SessionsDirectoryName);
        _historyPath = Path.Combine(dataDirectory, HistoryFileName);
        _minimumWatchedPercent = minimumWatchedPercent;
        Directory.CreateDirectory(_sessionsDirectory);
    }

    public static string MpvScriptPath => Path.Combine(
        Environment.GetFolderPath(Environment.SpecialFolder.ApplicationData),
        "mpv",
        "scripts",
        "playlistmaker-history.lua"
    );

    public PlaybackSessionManifest CreateSession(IReadOnlyList<MusicVideo> videos, string source)
    {
        var sessionId = Guid.NewGuid().ToString("N");
        var manifest = new PlaybackSessionManifest
        {
            SessionId = sessionId,
            RequestedAtUtc = DateTime.UtcNow,
            Entries = videos.Select((video, index) => new PlaybackSessionEntry
            {
                EntryId = Guid.NewGuid().ToString("N"),
                PlaylistPosition = index,
                PlaylistSize = videos.Count,
                SelectionSource = source,
                VideoPath = video.FilePath,
                AudioPath = video.Track.FilePath,
                Artist = video.Artist,
                Title = video.Title,
            }).ToList(),
        };
        WriteManifest(manifest);
        return manifest;
    }

    public IReadOnlyList<string> MpvArgumentsFor(PlaybackSessionManifest manifest) =>
    [
        $"--script-opt=playlistmaker_history-session_id={manifest.SessionId}",
        $"--script-opt=playlistmaker_history-manifest_path={ManifestPathFor(manifest.SessionId)}",
        $"--script-opt=playlistmaker_history-history_path={_historyPath}",
        $"--script-opt=playlistmaker_history-minimum_watched_percent={_minimumWatchedPercent}",
    ];

    public void RecordMpvProcess(PlaybackSessionManifest manifest, int processId)
    {
        manifest.MpvProcessId = processId;
        WriteManifest(manifest);
    }

    public void RecoverIncompleteSessions()
    {
        if (!Directory.Exists(_sessionsDirectory))
        {
            return;
        }

        foreach (var manifestPath in Directory.EnumerateFiles(_sessionsDirectory, "*.json"))
        {
            PlaybackSessionManifest? manifest;
            try
            {
                manifest = JsonSerializer.Deserialize<PlaybackSessionManifest>(
                    File.ReadAllText(manifestPath),
                    JsonOptions
                );
            }
            catch (JsonException)
            {
                Console.WriteLine($"Ignoring invalid playback session manifest: {manifestPath}");
                continue;
            }

            if (manifest is null || string.IsNullOrWhiteSpace(manifest.SessionId))
            {
                continue;
            }

            if (IsMpvStillRunning(manifest.MpvProcessId))
            {
                continue;
            }

            var events = EventsForSession(manifest.SessionId);
            var terminalEntryIds = events
                .Where(e => IsTerminal(e.Event))
                .Select(e => e.EntryId)
                .ToHashSet();
            var startedEntryIds = events
                .Where(e => e.Event == "started")
                .Select(e => e.EntryId)
                .ToHashSet();

            foreach (var entry in manifest.Entries.Where(e => !terminalEntryIds.Contains(e.EntryId)))
            {
                AppendEvent(CreateRecoveryEvent(
                    manifest,
                    entry,
                    startedEntryIds.Contains(entry.EntryId) ? "abandoned" : "not_started"
                ));
            }

            File.Delete(manifestPath);
        }
    }

    private IEnumerable<PlaybackHistoryEvent> EventsForSession(string sessionId)
    {
        if (!File.Exists(_historyPath))
        {
            return [];
        }

        return File.ReadLines(_historyPath)
            .Select(TryDeserializeEvent)
            .Where(e => e is not null && e.SessionId == sessionId)
            .Select(e => e!);
    }

    private void WriteManifest(PlaybackSessionManifest manifest)
    {
        File.WriteAllText(
            ManifestPathFor(manifest.SessionId),
            JsonSerializer.Serialize(manifest, JsonOptions),
            Encoding.UTF8
        );
    }

    private string ManifestPathFor(string sessionId) =>
        Path.Combine(_sessionsDirectory, $"{sessionId}.json");

    private void AppendEvent(PlaybackHistoryEvent historyEvent)
    {
        File.AppendAllText(
            _historyPath,
            JsonSerializer.Serialize(historyEvent, JsonOptions) + Environment.NewLine,
            Encoding.UTF8
        );
    }

    private static PlaybackHistoryEvent? TryDeserializeEvent(string line)
    {
        try
        {
            return JsonSerializer.Deserialize<PlaybackHistoryEvent>(line, JsonOptions);
        }
        catch (JsonException)
        {
            return null;
        }
    }

    private static bool IsMpvStillRunning(int? processId)
    {
        if (processId is null)
        {
            return false;
        }

        try
        {
            using var process = Process.GetProcessById(processId.Value);
            return !process.HasExited && process.ProcessName.Equals("mpv", StringComparison.OrdinalIgnoreCase);
        }
        catch (ArgumentException)
        {
            return false;
        }
    }

    private static bool IsTerminal(string eventName) => eventName is
        "completed" or "stopped" or "skipped" or "not_started" or "abandoned";

    private static PlaybackHistoryEvent CreateRecoveryEvent(
        PlaybackSessionManifest manifest,
        PlaybackSessionEntry entry,
        string eventName
    ) => new()
    {
        Event = eventName,
        EventAtUtc = DateTime.UtcNow,
        SessionId = manifest.SessionId,
        EntryId = entry.EntryId,
        PlaylistPosition = entry.PlaylistPosition,
        PlaylistSize = entry.PlaylistSize,
        SelectionSource = entry.SelectionSource,
        VideoPath = entry.VideoPath,
        AudioPath = entry.AudioPath,
        Artist = entry.Artist,
        Title = entry.Title,
        EndReason = "mpv-process-exited-without-terminal-event",
        CountedAsPlayed = false,
    };
}
