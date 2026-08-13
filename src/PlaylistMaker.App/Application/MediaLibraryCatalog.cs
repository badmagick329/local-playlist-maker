using PlaylistMaker.Core;
using PlaylistMaker.Exceptions;
using Raffinert.FuzzySharp;

namespace PlaylistMaker.Application;

public sealed class MediaLibraryCatalog
{
    private readonly IReadOnlyList<TrackGroup> _tracks;
    private readonly Dictionary<string, VideoVariant> _videosByPath;
    private readonly Dictionary<string, List<VideoVariant>> _videosByName;

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
    }

    public IReadOnlyList<TrackGroup> Tracks => _tracks;
    public IReadOnlyCollection<VideoVariant> Videos => _videosByPath.Values;

    public IReadOnlyList<TrackSearchResult> Search(LibraryQuery query)
    {
        var results = new List<TrackSearchResult>();
        foreach (var track in _tracks)
        {
            if (query.TrackDate is not null && !query.TrackDate.Contains(track.Track.Date))
            {
                continue;
            }

            var eligible = track.Variants
                .Where(v => query.Categories.Contains(v.Category))
                .Where(v => query.VideoDate is null || query.VideoDate.Contains(v.VideoDate))
                .ToList();
            if (eligible.Count == 0)
            {
                continue;
            }

            var score = SearchScore(query.SearchText, track, eligible);
            if (!string.IsNullOrWhiteSpace(query.SearchText) && score < 35)
            {
                continue;
            }

            results.Add(new TrackSearchResult(track, eligible, SelectDefault(eligible), score));
        }

        var sorted = ApplySort(results, query.Sort);
        return string.IsNullOrWhiteSpace(query.SearchText)
            ? sorted.ToList()
            : sorted.OrderByDescending(result => result.SearchScore)
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
        var official = variants.Where(v => v.Category == VideoCategory.MusicVideo).ToList();
        var candidates = official.Count > 0 ? official : variants;
        return candidates
            .OrderByDescending(v => v.VideoDate.Year)
            .ThenByDescending(v => v.VideoDate.Month ?? 0)
            .ThenByDescending(v => v.VideoDate.Day ?? 0)
            .ThenByDescending(v => v.ModifiedAtUtc)
            .ThenBy(v => v.VideoPath, PathIdentity.Comparer)
            .First();
    }

    private static int SearchScore(string searchText, TrackGroup track, IReadOnlyList<VideoVariant> variants)
    {
        if (string.IsNullOrWhiteSpace(searchText))
        {
            return 100;
        }

        var query = searchText.Trim();
        var candidates = new[] { track.Track.Artist, track.Track.Title, $"{track.Track.Artist} {track.Track.Title}" }
            .Concat(variants.Select(v => v.FileName));
        return candidates.Max(candidate =>
            candidate.Contains(query, StringComparison.InvariantCultureIgnoreCase)
                ? 100
                : Fuzz.WeightedRatio(query, candidate));
    }

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
