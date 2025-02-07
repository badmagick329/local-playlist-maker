using PlaylistMaker.Core;
using PlaylistMaker.Infrastructure;

namespace PlaylistMaker.View;

class VideoSorter
{
    private readonly IMusicVideoList _musicVideoList;
    private string _currentSorting;

    public VideoSorter(IMusicVideoList musicVideoList)
    {
        _musicVideoList = musicVideoList;
        _currentSorting = VideoListActions.OrderByNameDesc;
    }

    public bool TryUpdateSorting(string action)
    {
        switch (action)
        {
            case VideoListActions.OrderByName:
                _currentSorting = VideoListActions.OrderByName;
                return true;
            case VideoListActions.OrderByNameDesc:
                _currentSorting = VideoListActions.OrderByNameDesc;
                return true;
            case VideoListActions.OrderByMTime:
                _currentSorting = VideoListActions.OrderByMTime;
                return true;
            case VideoListActions.OrderByMTimeDesc:
                _currentSorting = VideoListActions.OrderByMTimeDesc;
                return true;
            case VideoListActions.OrderByArtist:
                _currentSorting = VideoListActions.OrderByArtist;
                return true;
            case VideoListActions.OrderByArtistDesc:
                _currentSorting = VideoListActions.OrderByArtistDesc;
                return true;
            case VideoListActions.OrderByReleaseDate:
                _currentSorting = VideoListActions.OrderByReleaseDate;
                return true;
            case VideoListActions.OrderByReleaseDateDesc:
                _currentSorting = VideoListActions.OrderByReleaseDateDesc;
                return true;
            default:
                return false;
        }
    }

    public List<string> ToSorted(List<string> videos) =>
        _currentSorting switch
        {
            VideoListActions.OrderByName => OrderListByName(videos, false),
            VideoListActions.OrderByNameDesc => OrderListByName(videos, true),
            VideoListActions.OrderByMTime => OrderListByMTime(videos, false),
            VideoListActions.OrderByMTimeDesc => OrderListByMTime(videos, true),
            VideoListActions.OrderByArtist => OrderListByArtist(videos, false),
            VideoListActions.OrderByArtistDesc => OrderListByArtist(videos, true),
            VideoListActions.OrderByReleaseDate => OrderListByReleaseDate(videos, false),
            VideoListActions.OrderByReleaseDateDesc => OrderListByReleaseDate(videos, true),
            _ => throw new ArgumentOutOfRangeException()
        };


    private static List<string> OrderListByName(List<string> videos, bool descending) => descending
        ? videos.OrderByDescending(v => v).ToList()
        : videos.OrderBy(v => v).ToList();

    private List<string> OrderListByMTime(List<string> videos, bool descending) => descending
        ? videos.OrderByDescending(v => File.GetLastWriteTime(_musicVideoList.VideoPathFor(v))).ToList()
        : videos.OrderBy(v => File.GetLastWriteTime(_musicVideoList.VideoPathFor(v))).ToList();

    private List<string> OrderListByArtist(List<string> videos, bool descending)
    {
        var videoNameAndMusicVideo = new List<(string videoName, MusicVideo musicVideo)>();
        videos.ForEach(videoName =>
            videoNameAndMusicVideo.Add((videoName, _musicVideoList.MusicVideoFor(videoName))));

        var query = descending
            ? videoNameAndMusicVideo.OrderByDescending(v => v.musicVideo.Artist)
            : videoNameAndMusicVideo.OrderBy(v => v.musicVideo.Artist);

        return query
            .ThenBy(v => v.musicVideo.FullDate)
            .Select(v => v.videoName).ToList();
    }

    private List<string> OrderListByReleaseDate(List<string> videos, bool descending) => descending
        ? videos.OrderByDescending(v => _musicVideoList.MusicVideoFor(v).Track.FullDate).ToList()
        : videos.OrderBy(v => _musicVideoList.MusicVideoFor(v).Track.FullDate).ToList();
}