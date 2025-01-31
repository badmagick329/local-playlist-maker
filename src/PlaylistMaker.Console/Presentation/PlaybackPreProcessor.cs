using PlaylistMaker.Application;
using PlaylistMaker.Core;
using PlaylistMaker.Infrastructure;

namespace PlaylistMaker.Presentation;

class PlaybackPreProcessor
{
    private readonly IMusicVideoList _musicVideoList;
    private readonly IPrimitivesEnquirer _userInputReader;
    private bool _randomPlay;
    private int _videoMultiplier = 1;
    private bool _oneVideoPerTrack;
    private int _maxInPlaylist;

    public PlaybackPreProcessor(IMusicVideoList musicVideoList, IPrimitivesEnquirer userInputReader)
    {
        _musicVideoList = musicVideoList;
        _userInputReader = userInputReader;
    }

    public bool TryUpdatePlayMethod(List<string> choices)
    {
        if (choices.Count != 1)
        {
            return false;
        }

        var action = choices[0];
        switch (action)
        {
            case VideoListActions.ToggleRandomPlay:
                _randomPlay = !_randomPlay;
                Console.WriteLine($"Random play: {_randomPlay}");
                return true;
            case VideoListActions.MultiAdd:
                var maybeVideoMultiplier =
                    _userInputReader.AskInt($"Enter the number of times to repeat a video (max: 10)");
                _videoMultiplier = maybeVideoMultiplier is null
                    ? 1
                    : Math.Max(1, Math.Min(maybeVideoMultiplier.Value, 10));
                Console.WriteLine($"Video multiplier: {_videoMultiplier}");
                return true;
            case VideoListActions.ToggleOneVideoPerTrack:
                _oneVideoPerTrack = !_oneVideoPerTrack;
                Console.WriteLine($"One video per track: {_oneVideoPerTrack}");
                return true;
            case VideoListActions.MaxInPlaylist:
                var maybeSongNum =
                    _userInputReader.AskInt($"Enter the max number of songs in playlist (0=no limit)");
                _maxInPlaylist = maybeSongNum is null ? 0 : Math.Max(0, maybeSongNum.Value);
                return true;
            // TODO: Move this into another handler?
            case VideoListActions.PrintTopArtistCount:
                var topArtists = _musicVideoList.TopArtists(30);
                var artistCountStrings = new List<string>();
                foreach (var (artist, count) in topArtists)
                {
                    artistCountStrings.Add($"{artist}: {count}");
                }

                Console.WriteLine(string.Join(" __ ", artistCountStrings));
                return true;
        }

        return false;
    }

    public List<string> Process(List<string> videos)
    {
        var temp = new List<string>();
        // Handle video multiplier
        Enumerable.Range(0, _videoMultiplier).ToList().ForEach(_ => temp.AddRange(videos));
        videos = temp;

        // Handle random play
        if (_randomPlay)
        {
            videos = videos.OrderBy(_ => Guid.NewGuid()).ToList();
        }

        // Handle one video per track
        if (!_oneVideoPerTrack)
        {
            return videos;
        }

        // Create video to flac mapping
        Dictionary<string, List<string>> flacToVideosMap = [];
        foreach (var video in videos)
        {
            var audioPath = _musicVideoList.AudioPathFor(video);
            if (flacToVideosMap.TryGetValue(audioPath, out var list))
            {
                list.Add(video);
            }
            else
            {
                flacToVideosMap[audioPath] = new List<string> { video };
            }
        }

        // Pick videos at random
        var rng = new Random();
        var query = flacToVideosMap.Values
            .Select(associatedVideos => associatedVideos[rng.Next(associatedVideos.Count)]);

        // Handle max in playlist
        if (_maxInPlaylist > 0)
        {
            query = query.Take(_maxInPlaylist);
        }

        return query.ToList();
    }
}