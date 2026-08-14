using PlaylistMaker.Application;
using PlaylistMaker.Core;

namespace PlaylistMaker.Tui;

public sealed record LibraryRow(TrackSearchResult Result, VideoVariant? Variant = null)
{
    public bool IsTrack => Variant is null;
}

public sealed class TuiState
{
    private readonly HashSet<string> _expandedTracks = new(PathIdentity.Comparer);

    public TuiState(MediaLibraryCatalog catalog, PlaybackHistoryReader historyReader)
    {
        Catalog = catalog;
        HistoryReader = historyReader;
        Categories = [VideoCategory.MusicVideo];
        RefreshHistory();
        RefreshResults();
    }

    public MediaLibraryCatalog Catalog { get; private set; }
    public PlaybackHistoryReader HistoryReader { get; }
    public PlaylistDraft Queue { get; } = new();
    public PlaybackHistoryIndex History { get; private set; } = new();
    public HashSet<VideoCategory> Categories { get; }
    public string SearchText { get; private set; } = string.Empty;
    public DateRange? TrackDate { get; private set; }
    public DateRange? VideoDate { get; private set; }
    public LibrarySort Sort { get; private set; } = LibrarySort.ModifiedDescending;
    public PlaybackOptions PlaybackOptions { get; set; } = new();
    public IReadOnlyList<TrackSearchResult> Results { get; private set; } = [];
    public IReadOnlyList<LibraryRow> Rows { get; private set; } = [];

    public void SetSearch(string text)
    {
        SearchText = text;
        RefreshResults();
    }

    public void ReplaceCatalog(MediaLibraryCatalog catalog)
    {
        Catalog = catalog;
        _expandedTracks.Clear();
        RefreshResults();
    }

    public void SetSort(LibrarySort sort)
    {
        Sort = sort;
        RefreshResults();
    }

    public void SetTrackDate(DateRange? range)
    {
        TrackDate = range;
        RefreshResults();
    }

    public void SetVideoDate(DateRange? range)
    {
        VideoDate = range;
        RefreshResults();
    }

    public void ToggleCategory(VideoCategory category)
    {
        if (!Categories.Add(category))
        {
            Categories.Remove(category);
        }

        RefreshResults();
    }

    public void ResetFilters()
    {
        Categories.Clear();
        Categories.Add(VideoCategory.MusicVideo);
        TrackDate = null;
        VideoDate = null;
        RefreshResults();
    }

    public void ToggleExpansion(int rowIndex)
    {
        var row = RowAt(rowIndex);
        if (row is null)
        {
            return;
        }

        var trackId = row.Result.Group.Id;
        if (!_expandedTracks.Add(trackId))
        {
            _expandedTracks.Remove(trackId);
        }

        BuildRows();
    }

    public bool SetExpanded(int rowIndex, bool expanded)
    {
        var row = RowAt(rowIndex);
        if (row is null)
        {
            return false;
        }

        var changed = expanded
            ? _expandedTracks.Add(row.Result.Group.Id)
            : _expandedTracks.Remove(row.Result.Group.Id);
        if (changed)
        {
            BuildRows();
        }

        return changed;
    }

    public bool ToggleQueue(int rowIndex)
    {
        var row = RowAt(rowIndex);
        return row is not null && Queue.Toggle(row.Variant ?? row.Result.DefaultVariant);
    }

    public LibraryRow? RowAt(int index) => index >= 0 && index < Rows.Count ? Rows[index] : null;

    public bool IsExpanded(string trackId) => _expandedTracks.Contains(trackId);

    public void QueueVisibleTracks() => Queue.AddRange(Results.Select(result => result.DefaultVariant));

    public void QueueMatchingVideos() => Queue.AddRange(Results.SelectMany(result => result.EligibleVariants));

    public void RefreshHistory()
    {
        History = HistoryReader.Read();
    }

    public string DetailsFor(LibraryRow? row)
    {
        if (row is null)
        {
            return "No item selected.";
        }

        var variant = row.Variant ?? row.Result.DefaultVariant;
        var summary = row.Variant is null
            ? History.ForTrack(row.Result.Group.Track.FilePath)
            : History.ForVideo(variant.VideoPath);
        var recent = summary.RecentEvents.Count == 0
            ? "No playback history."
            : string.Join(Environment.NewLine, summary.RecentEvents.Select(item =>
                $"{item.Raw.EventAtUtc.ToLocalTime():yyyy-MM-dd HH:mm}  {item.Outcome,-11} "
                + $"{(item.WatchedPercent is null ? "" : $"{item.WatchedPercent:0.#}%")}"));

        return $"{variant.Artist} — {variant.Title}{Environment.NewLine}"
               + $"Track release: {variant.Video.Track.Date}{Environment.NewLine}"
               + $"Video: {variant.FileName}{Environment.NewLine}"
               + $"Video date/category: {variant.VideoDate} / {variant.Category}{Environment.NewLine}"
               + $"Plays: {summary.PlayedCount}   Completed: {summary.CompletedCount}   "
               + $"Stopped: {summary.StoppedCount}   Skips: {summary.SkippedCount}{Environment.NewLine}"
               + $"Last played: {summary.LastPlayedAtUtc?.ToLocalTime().ToString("yyyy-MM-dd HH:mm") ?? "never"}{Environment.NewLine}{Environment.NewLine}"
               + $"Recent:{Environment.NewLine}{recent}";
    }

    private void RefreshResults()
    {
        Results = Catalog.Search(new LibraryQuery
        {
            SearchText = SearchText,
            Categories = Categories,
            TrackDate = TrackDate,
            VideoDate = VideoDate,
            Sort = Sort,
        });
        BuildRows();
    }

    private void BuildRows()
    {
        var rows = new List<LibraryRow>(Results.Count + _expandedTracks.Count * 2);
        foreach (var result in Results)
        {
            rows.Add(new LibraryRow(result));
            if (_expandedTracks.Contains(result.Group.Id))
            {
                rows.AddRange(result.EligibleVariants.Select(variant => new LibraryRow(result, variant)));
            }
        }

        Rows = rows;
    }
}
