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
        var process = new Process
        {
            StartInfo = new ProcessStartInfo
            {
                FileName = Config.PlaylistCommand.Program,
                Arguments = Config.PlaylistCommand.ParsedArguments(),
                UseShellExecute = true,
                CreateNoWindow = false
            }
        };
        process.Start();
    }

    public void CreateAndPlay(List<string> trackPaths)
    {
        if (trackPaths.Count < 1)
        {
            return;
        }

        if (trackPaths.Count == 1 && Config.SingleFileCommand is not null)
        {
            PlaySingle(trackPaths[0]);
            return;
        }

        PlaylistName = GenerateNonRandomName();
        Config.PlaylistCommand.SetArgumentSubstitution(Config.PlaylistArgumentTemplate, PlaylistName);
        File.WriteAllLines(PlaylistName, trackPaths, encoding: Encoding.UTF8);
        Play(PlaylistName);
    }

    private void PlaySingle(string trackPath)
    {
        if (Config.SingleFileCommand is null)
        {
            throw new InvalidOperationException("Single file command not set");
        }

        trackPath = $"\"{trackPath}\"";
        var process = new Process
        {
            StartInfo = new ProcessStartInfo
            {
                FileName = Config.SingleFileCommand.Program,
                Arguments = Config.SingleFileCommand.ArgumentsWith(trackPath),
                UseShellExecute = true,
                CreateNoWindow = false
            }
        };
        process.Start();
    }

    private string GenerateTimestampName()
    {
        var now = DateTime.UtcNow
            .ToString(CultureInfo.InvariantCulture)
            .Replace(':', '_')
            .Replace(' ', '_')
            .Replace('\\', '-')
            .Replace('/', '-');
        return Path.Combine(Config.PlaylistDirectory, $"{now}{Config.PlaylistSuffix}");
    }

    private string GenerateNonRandomName() =>
        Path.Combine(Config.PlaylistDirectory, $"playlist_{Config.PlaylistSuffix}");
}