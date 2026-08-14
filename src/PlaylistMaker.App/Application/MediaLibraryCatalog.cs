using PlaylistMaker.Core;
using PlaylistMaker.Exceptions;
using Raffinert.FuzzySharp.SimilarityRatio.Scorer.StrategySensitive;

namespace PlaylistMaker.Application;

public sealed class MediaLibraryCatalog
{
    private readonly IReadOnlyList<TrackGroup> _tracks;
    private readonly Dictionary<string, VideoVariant> _videosByPath;
    private readonly Dictionary<string, List<VideoVariant>> _videosByName;
    private readonly Dictionary<string, string[]> _trackSearchCandidates;
    private readonly Dictionary<string, string[]> _videoSearchCandidates;

    public MediaLibraryCatalog(IVorbisReader reader, IReadOnlyDictionary<string, string> videoAudioMap)
    {
        var variants = new List<VideoVariant>(videoAudioMap.Count);
        foreach (var (videoPath, audioPath) in videoAudioMap)
        {
            var vorbis = reader.VorbisDataFor(audioPath)
                ?? throw new VideoPlayerException($"No vorbis data found for {audioPath}");
            var track = TrackFactory.FromVorbisData(vorbis)
                ?? throw new VideoPlayerException($"Track is null. {audioPath}");
            var video = new MusicVideo(videoPath, track);
            variants.Add(new VideoVariant(
                PathIdentity.Normalize(videoPath),
                video,
                VideoCategoryClassifier.Classify(video),
                File.Exists(videoPath) ? File.GetLastWriteTimeUtc(videoPath) : DateTime.MinValue
            ));
        }

        var consolidated = variants
            .GroupBy(variant => variant.Id, PathIdentity.Comparer)
            .Select(group => group.Last())
            .ToList();
        _videosByPath = consolidated.ToDictionary(v => v.Id, PathIdentity.Comparer);
        _videosByName = consolidated
            .GroupBy(v => v.FileName, PathIdentity.Comparer)
            .ToDictionary(g => g.Key, g => g.ToList(), PathIdentity.Comparer);
        _tracks = consolidated
            .GroupBy(v => PathIdentity.Normalize(v.AudioPath), PathIdentity.Comparer)
            .Select(group => new TrackGroup(
                group.Key,
                group.First().Video.Track,
                group.OrderByDescending(v => v.VideoDate.Year)
                    .ThenByDescending(v => v.VideoDate.Month ?? 0)
                    .ThenByDescending(v => v.VideoDate.Day ?? 0)
                    .ThenBy(v => v.VideoPath, PathIdentity.Comparer)
                    .ToList()
            ))
            .ToList();
        _trackSearchCandidates = _tracks.ToDictionary(
            track => track.Id,
            track => new[]
            {
                NormalizeSearchText(track.Track.Artist),
                NormalizeSearchText(track.Track.Title),
                NormalizeSearchText($"{track.Track.Artist} {track.Track.Title}"),
            },
            PathIdentity.Comparer
        );
        _videoSearchCandidates = consolidated.ToDictionary(
            video => video.Id,
            video => BuildVideoSearchCandidates(video.FileName),
            PathIdentity.Comparer
        );
    }

    public IReadOnlyList<TrackGroup> Tracks => _tracks;
    public IReadOnlyCollection<VideoVariant> Videos => _videosByPath.Values;

    public IReadOnlyList<TrackSearchResult> Search(LibraryQuery query)
    {
        var searchText = NormalizeSearchText(query.SearchText.Trim());
        using var fuzzyScorer = searchText.Length == 0
            ? null
            : new CachedDefaultRatioScorer(searchText);
        var results = new List<TrackSearchResult>(_tracks.Count);
        foreach (var track in _tracks)
        {
            if (query.TrackDate is not null && !query.TrackDate.Contains(track.Track.Date))
            {
                continue;
            }

            var eligible = new List<VideoVariant>(track.Variants.Count);
            foreach (var variant in track.Variants)
            {
                if (query.Categories.Contains(variant.Category)
                    && (query.VideoDate is null || query.VideoDate.Contains(variant.VideoDate)))
                {
                    eligible.Add(variant);
                }
            }
            if (eligible.Count == 0)
            {
                continue;
            }

            var score = SearchScore(searchText, fuzzyScorer, track, eligible);
            if (searchText.Length > 0 && score < 35)
            {
                continue;
            }

            results.Add(new TrackSearchResult(track, eligible, SelectDefault(eligible), score));
        }

        return searchText.Length == 0
            ? ApplySort(results, query.Sort).ToList()
            : results.OrderByDescending(result => result.SearchScore)
                .ThenBy(result => result.Group.Track.Artist)
                .ThenBy(result => result.Group.Track.Title)
                .ToList();
    }

    public VideoVariant? FindByPath(string path)
    {
        var normalized = PathIdentity.Normalize(path);
        return _videosByPath.GetValueOrDefault(normalized);
    }

    public IReadOnlyList<VideoVariant> FindByFileName(string name) =>
        _videosByName.TryGetValue(Path.GetFileName(name), out var matches) ? matches : [];

    public IReadOnlyList<(string Artist, int Count)> TopArtists(int count) => _videosByPath.Values
        .GroupBy(v => v.Artist, StringComparer.InvariantCultureIgnoreCase)
        .Select(g => (g.Key, g.Count()))
        .OrderByDescending(item => item.Item2)
        .ThenBy(item => item.Key)
        .Take(count)
        .ToList();

    public static VideoVariant SelectDefault(IReadOnlyList<VideoVariant> variants)
    {
        if (variants.Count == 0)
        {
            throw new ArgumentException("At least one video variant is required.", nameof(variants));
        }

        var hasOfficial = false;
        VideoVariant? selected = null;
        foreach (var variant in variants)
        {
            var isOfficial = variant.Category == VideoCategory.MusicVideo;
            if (selected is null
                || (isOfficial && !hasOfficial)
                || (isOfficial == hasOfficial && IsPreferredDefault(variant, selected)))
            {
                selected = variant;
                hasOfficial = isOfficial;
            }
        }

        return selected!;
    }

    private static bool IsPreferredDefault(VideoVariant candidate, VideoVariant current)
    {
        var dateComparison = CompareVideoDate(candidate, current);
        if (dateComparison != 0)
        {
            return dateComparison > 0;
        }

        var modifiedComparison = candidate.ModifiedAtUtc.CompareTo(current.ModifiedAtUtc);
        return modifiedComparison != 0
            ? modifiedComparison > 0
            : PathIdentity.Comparer.Compare(candidate.VideoPath, current.VideoPath) < 0;
    }

    private static int CompareVideoDate(VideoVariant left, VideoVariant right)
    {
        var comparison = left.VideoDate.Year.CompareTo(right.VideoDate.Year);
        if (comparison != 0)
        {
            return comparison;
        }

        comparison = (left.VideoDate.Month ?? 0).CompareTo(right.VideoDate.Month ?? 0);
        return comparison != 0
            ? comparison
            : (left.VideoDate.Day ?? 0).CompareTo(right.VideoDate.Day ?? 0);
    }

    private int SearchScore(
        string searchText,
        CachedDefaultRatioScorer? fuzzyScorer,
        TrackGroup track,
        IReadOnlyList<VideoVariant> variants
    )
    {
        if (searchText.Length == 0)
        {
            return 100;
        }

        var score = ScoreCandidates(searchText, fuzzyScorer!, _trackSearchCandidates[track.Id]);
        if (score == 100)
        {
            return score;
        }

        foreach (var variant in variants)
        {
            score = Math.Max(
                score,
                ScoreCandidates(searchText, fuzzyScorer!, _videoSearchCandidates[variant.Id])
            );
            if (score == 100)
            {
                return score;
            }
        }

        return score;
    }

    private static int ScoreCandidates(
        string searchText,
        CachedDefaultRatioScorer fuzzyScorer,
        IReadOnlyList<string> candidates
    )
    {
        var score = 0;
        foreach (var candidate in candidates)
        {
            if (candidate.Contains(searchText, StringComparison.Ordinal))
            {
                return 100;
            }
            score = Math.Max(score, fuzzyScorer.Score(candidate));
        }
        return score;
    }

    private static string[] BuildVideoSearchCandidates(string fileName)
    {
        var normalized = NormalizeSearchText(fileName);
        return new[] { normalized }
            .Concat(normalized.Split(
                [' ', '-', '_', '.', '(', ')', '[', ']', '{', '}'],
                StringSplitOptions.RemoveEmptyEntries | StringSplitOptions.TrimEntries
            ))
            .Distinct(StringComparer.Ordinal)
            .ToArray();
    }

    private static string NormalizeSearchText(string value) => value.ToLowerInvariant();

    private static IOrderedEnumerable<TrackSearchResult> ApplySort(
        IEnumerable<TrackSearchResult> source,
        LibrarySort sort
    ) => sort switch
    {
        LibrarySort.NameAscending => source.OrderBy(r => r.Group.Track.Title).ThenBy(r => r.Group.Track.Artist),
        LibrarySort.NameDescending => source.OrderByDescending(r => r.Group.Track.Title).ThenBy(r => r.Group.Track.Artist),
        LibrarySort.ArtistAscending => source.OrderBy(r => r.Group.Track.Artist).ThenBy(r => r.Group.Track.FullDate),
        LibrarySort.ArtistDescending => source.OrderByDescending(r => r.Group.Track.Artist).ThenBy(r => r.Group.Track.FullDate),
        LibrarySort.TrackReleaseAscending => source.OrderBy(r => r.Group.Track.FullDate).ThenBy(r => r.Group.Track.Artist),
        LibrarySort.TrackReleaseDescending => source.OrderByDescending(r => r.Group.Track.FullDate).ThenBy(r => r.Group.Track.Artist),
        LibrarySort.VideoDateAscending => source.OrderBy(r => r.DefaultVariant.VideoDate.Year)
            .ThenBy(r => r.DefaultVariant.VideoDate.Month ?? 0).ThenBy(r => r.DefaultVariant.VideoDate.Day ?? 0),
        LibrarySort.VideoDateDescending => source.OrderByDescending(r => r.DefaultVariant.VideoDate.Year)
            .ThenByDescending(r => r.DefaultVariant.VideoDate.Month ?? 0).ThenByDescending(r => r.DefaultVariant.VideoDate.Day ?? 0),
        LibrarySort.ModifiedAscending => source.OrderBy(r => r.DefaultVariant.ModifiedAtUtc),
        LibrarySort.ModifiedDescending => source.OrderByDescending(r => r.DefaultVariant.ModifiedAtUtc),
        _ => source.OrderByDescending(r => r.SearchScore).ThenBy(r => r.Group.Track.Artist),
    };
}
