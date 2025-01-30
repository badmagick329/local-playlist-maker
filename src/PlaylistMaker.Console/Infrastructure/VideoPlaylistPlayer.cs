using System.Diagnostics;
using System.Globalization;
using System.Text;
using PlaylistMaker.Core;

namespace PlaylistMaker.Infrastructure;

public class VideoPlaylistPlayer : IPlaylistPlayer
{
    private string _playlistName;

    public void Play(string playlistPath)
    {
        var process = new Process
        {
            StartInfo = new ProcessStartInfo
            {
                FileName = "mpv.exe",
                Arguments = $"--fullscreen --playlist={playlistPath}",
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

        _playlistName = GenerateTimestampName();
        File.WriteAllLines(_playlistName, trackPaths, encoding: Encoding.UTF8);
        Play(_playlistName);
    }

    private static string GenerateTimestampName()
    {
        var now = DateTime.UtcNow
            .ToString(CultureInfo.InvariantCulture)
            .Replace(':', '_')
            .Replace(' ', '_')
            .Replace('\\', '-')
            .Replace('/', '-');
        return Path.Combine("data", $"{now}_videos.txt");
    }
}