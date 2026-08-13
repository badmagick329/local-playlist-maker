using System.Reflection;
using System.Text;

namespace PlaylistMaker.Infrastructure;

public enum MpvScriptStatus
{
    Current,
    Missing,
    Outdated,
}

public sealed class MpvScriptManager
{
    private const string ResourceSuffix = "playlistmaker-history.lua";
    private readonly string _installedPath;

    public MpvScriptManager(string? installedPath = null) =>
        _installedPath = installedPath ?? PlaybackHistoryService.MpvScriptPath;

    public string InstalledPath => _installedPath;

    public MpvScriptStatus Status()
    {
        if (!File.Exists(InstalledPath))
        {
            return MpvScriptStatus.Missing;
        }

        return Normalize(File.ReadAllText(InstalledPath)) == Normalize(BundledScript())
            ? MpvScriptStatus.Current
            : MpvScriptStatus.Outdated;
    }

    public void InstallOrUpdate()
    {
        var directory = Path.GetDirectoryName(InstalledPath)
            ?? throw new InvalidOperationException("The mpv script directory could not be determined.");
        Directory.CreateDirectory(directory);
        File.WriteAllText(InstalledPath, BundledScript(), new UTF8Encoding(false));
    }

    public string BundledScript()
    {
        var assembly = typeof(MpvScriptManager).Assembly;
        var resourceName = assembly.GetManifestResourceNames()
            .Single(name => name.EndsWith(ResourceSuffix, StringComparison.Ordinal));
        using var stream = assembly.GetManifestResourceStream(resourceName)
            ?? throw new InvalidOperationException("The bundled mpv script could not be loaded.");
        using var reader = new StreamReader(stream, Encoding.UTF8);
        return reader.ReadToEnd();
    }

    private static string Normalize(string content) => content.Replace("\r\n", "\n").Trim();
}
