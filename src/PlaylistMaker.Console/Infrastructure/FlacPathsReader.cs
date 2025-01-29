using System.Text.Json;
using PlaylistMaker.Common;
using PlaylistMaker.Core;
using PlaylistMaker.Exceptions;

namespace PlaylistMaker.Infrastructure;

public class FlacPathsReader : IFlacPathsReader
{
    private string PlaylistPath { get; init; }
    private string MvPlayerListPath { get; init; }

    public FlacPathsReader(string playlistPath, string mvPlayerListPath)
    {
        PlaylistPath = playlistPath;
        MvPlayerListPath = mvPlayerListPath;
        if (!File.Exists(playlistPath))
        {
            throw new FlacReaderException("Playlist file does not exist");
        }

        // TODO: Replace with the map reader?
        if (!File.Exists(mvPlayerListPath))
        {
            throw new FlacReaderException("MVPlayer list file does not exist");
        }
    }

    public List<string> ReadFlacPaths()
    {
        var flacsFromAudioPlaylist = File.ReadAllLines(PlaylistPath)
            .Where(l => !string.IsNullOrEmpty(l.Trim()) && !l.StartsWith('#'))
            .Select(PathOperations.FixFlacPath)
            .Distinct()
            .ToList();
        var mvPlayerListData = JsonSerializer.Deserialize<Dictionary<string, string>>(
            File.ReadAllText(MvPlayerListPath), new JsonSerializerOptions
            {
                PropertyNameCaseInsensitive = true
            }) ?? throw new FlacReaderException("MVPlayer list file is empty");
        var flacsFromMvPlayerList =
            mvPlayerListData.Values
                .Select(PathOperations.FixFlacPath)
                .Distinct()
                .ToList();
        var flacPaths = flacsFromAudioPlaylist.Intersect(flacsFromMvPlayerList).ToList();
        if (flacPaths.Count == 0)
        {
            throw new FlacReaderException("No flac files found in both lists");
        }

        EnsureValidPathsElseExit(flacPaths);

        return flacPaths;
    }

    public static void EnsureValidPathsElseExit(List<string> paths)
    {
        var notFound = paths.Where(path => !File.Exists(path)).ToList();

        if (notFound.Count <= 0)
        {
            return;
        }

        foreach (var file in notFound)
        {
            Console.WriteLine($"File not found: {file}");
        }

        Environment.Exit(1);
    }
}