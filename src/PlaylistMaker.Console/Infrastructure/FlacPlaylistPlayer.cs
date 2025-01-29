using System.Diagnostics;
using System.Globalization;
using System.Text;
using PlaylistMaker.Application;
using PlaylistMaker.Core;

namespace PlaylistMaker.Infrastructure;

public class FlacPlaylistPlayer : IPlaylistPlayer
{
    private string _playlistName;

    public void Play(string playlistPath)
    {
        var process = new Process
        {
            StartInfo = new ProcessStartInfo
            {
                FileName = @"C:\Program Files\foobar2000\foobar2000.exe",
                Arguments = playlistPath,
                WindowStyle = ProcessWindowStyle.Hidden,
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
        return Path.Combine("data", $"{now}_audios.m3u8");
    }
}