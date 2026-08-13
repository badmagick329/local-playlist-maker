namespace PlaylistMaker.Tui;

internal sealed class HistoryFileWatcher : IDisposable
{
    private readonly FileSystemWatcher _watcher;
    private readonly System.Threading.Timer _timer;
    private readonly Action _changed;

    public HistoryFileWatcher(string historyPath, Action changed)
    {
        _changed = changed;
        var directory = Path.GetDirectoryName(Path.GetFullPath(historyPath))!;
        Directory.CreateDirectory(directory);
        _timer = new System.Threading.Timer(_ => _changed(), null, Timeout.Infinite, Timeout.Infinite);
        _watcher = new FileSystemWatcher(directory, Path.GetFileName(historyPath))
        {
            NotifyFilter = NotifyFilters.FileName | NotifyFilters.LastWrite | NotifyFilters.Size,
            EnableRaisingEvents = true,
        };
        _watcher.Changed += OnChanged;
        _watcher.Created += OnChanged;
        _watcher.Renamed += OnChanged;
    }

    private void OnChanged(object? sender, FileSystemEventArgs eventArgs) =>
        _timer.Change(TimeSpan.FromMilliseconds(350), Timeout.InfiniteTimeSpan);

    public void Dispose()
    {
        _watcher.Dispose();
        _timer.Dispose();
    }
}
