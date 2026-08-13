using PlaylistMaker.Core;

namespace PlaylistMaker.Application;

public sealed record PlaybackOptions(
    bool Shuffle = false,
    int MaximumItems = 0,
    int RepeatEach = 1,
    bool OneVideoPerTrack = false
)
{
    public PlaybackOptions Validate() => this with
    {
        MaximumItems = Math.Max(0, MaximumItems),
        RepeatEach = Math.Clamp(RepeatEach, 1, 10),
    };
}

public sealed record PlaybackRequest(
    IReadOnlyList<MusicVideo> Videos,
    string Source = "interactive-selection"
);

public sealed class PlaybackPlanner
{
    private readonly Random _random;

    public PlaybackPlanner(Random? random = null) => _random = random ?? Random.Shared;

    public PlaybackRequest Create(
        IReadOnlyList<VideoVariant> draft,
        PlaybackOptions options,
        string source = "interactive-selection"
    )
    {
        options = options.Validate();
        IEnumerable<VideoVariant> planned = draft;

        if (options.OneVideoPerTrack)
        {
            planned = planned
                .GroupBy(item => PathIdentity.Normalize(item.AudioPath), PathIdentity.Comparer)
                .Select(group => group.ElementAt(_random.Next(group.Count())));
        }

        var expanded = planned
            .SelectMany(item => Enumerable.Repeat(item, options.RepeatEach))
            .ToList();
        if (options.Shuffle)
        {
            expanded = expanded.OrderBy(_ => _random.Next()).ToList();
        }

        if (options.MaximumItems > 0)
        {
            expanded = expanded.Take(options.MaximumItems).ToList();
        }

        return new PlaybackRequest(expanded.Select(item => item.Video).ToList(), source);
    }
}
