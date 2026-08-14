using PlaylistMaker.Infrastructure;

namespace PlaylistMaker.App.Tests;

public class MpvScriptManagerTests : IDisposable
{
    private readonly string _directory = Path.Combine(Path.GetTempPath(), Guid.NewGuid().ToString("N"));

    [Fact]
    public void DetectsMissingOutdatedAndCurrentScriptAndInstallsExplicitly()
    {
        var path = Path.Combine(_directory, "mpv", "scripts", "playlistmaker-history.lua");
        var manager = new MpvScriptManager(path);
        Assert.Equal(MpvScriptStatus.Missing, manager.Status());

        Directory.CreateDirectory(Path.GetDirectoryName(path)!);
        File.WriteAllText(path, "old");
        Assert.Equal(MpvScriptStatus.Outdated, manager.Status());

        manager.InstallOrUpdate();
        Assert.Equal(MpvScriptStatus.Current, manager.Status());
        var installed = File.ReadAllText(path);
        Assert.Contains("playlistmaker-history-version: 3", installed);
        Assert.Contains("demuxer-start-time", installed);
        Assert.Contains("rawDurationSeconds", installed);
    }

    public void Dispose()
    {
        if (Directory.Exists(_directory)) Directory.Delete(_directory, true);
    }
}
