using System.Diagnostics;
using System.Globalization;
using System.Text;
using PlaylistMaker.Core;

namespace PlaylistMaker.Infrastructure;

public class PlaylistPlayer : IPlaylistPlayer
{
    private string PlaylistName { get; set; }
    private PlaylistPlayerConfig Config { get; }

    public PlaylistPlayer(PlaylistPlayerConfig config) => Config = config;

    public void Play(string playlistPath)
    {
        StartProcess(Config.PlaylistCommand);
    }

    public int? CreateAndPlay(
        List<string> trackPaths,
        IReadOnlyList<string>? additionalArguments = null
    )
    {
        trackPaths = trackPaths.Where(tp => Path.Exists(tp)).ToList();
        if (trackPaths.Count < 1)
        {
            return null;
        }

        if (trackPaths.Count == 1 && Config.SingleFileCommand is not null)
        {
            return PlaySingle(trackPaths[0], additionalArguments);
        }

        PlaylistName = GenerateNonRandomName();
        Config.PlaylistCommand.SetArgumentSubstitution(
            Config.PlaylistArgumentTemplate,
            PlaylistName
        );
        File.WriteAllLines(PlaylistName, trackPaths, encoding: Encoding.UTF8);
        return StartProcess(Config.PlaylistCommand, additionalArguments);
    }

    private int PlaySingle(string trackPath, IReadOnlyList<string>? additionalArguments)
    {
        if (Config.SingleFileCommand is null)
        {
            throw new InvalidOperationException("Single file command not set");
        }

        return StartProcess(Config.SingleFileCommand, additionalArguments, trackPath);
    }

    private static int StartProcess(
        ICliCommand command,
        IReadOnlyList<string>? additionalArguments = null,
        string? trailingArgument = null
    )
    {
        var process = new Process
        {
            StartInfo = new ProcessStartInfo
            {
                FileName = command.Program,
                UseShellExecute = false,
                CreateNoWindow = false,
            },
        };
        foreach (var argument in command.ParsedArgumentList())
        {
            process.StartInfo.ArgumentList.Add(argument);
        }

        if (additionalArguments is not null)
        {
            foreach (var argument in additionalArguments)
            {
                process.StartInfo.ArgumentList.Add(argument);
            }
        }

        if (!string.IsNullOrEmpty(trailingArgument))
        {
            process.StartInfo.ArgumentList.Add(trailingArgument);
        }

        process.Start();
        return process.Id;
    }

    private string GenerateTimestampName()
    {
        var now = DateTime
            .UtcNow.ToString(CultureInfo.InvariantCulture)
            .Replace(':', '_')
            .Replace(' ', '_')
            .Replace('\\', '-')
            .Replace('/', '-');
        return Path.Combine(Config.PlaylistDirectory, $"{now}{Config.PlaylistSuffix}");
    }

    private string GenerateNonRandomName() =>
        Path.Combine(Config.PlaylistDirectory, $"playlist_{Config.PlaylistSuffix}");
}
