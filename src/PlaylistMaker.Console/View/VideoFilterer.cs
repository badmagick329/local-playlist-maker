using PlaylistMaker.Application;
using PlaylistMaker.Core;
using PlaylistMaker.Core.VideoFilters;
using PlaylistMaker.Infrastructure;

namespace PlaylistMaker.View;

class VideoFilterer
{
    private readonly IDateRangeEnquirer _dateRangeEnquirer;

    private readonly IMusicVideoList _musicVideoList;
    private readonly CategoryFilters _categoryFilters;
    private readonly DateFilters _dateFilters;

    public VideoFilterer(IMusicVideoList musicVideoList, IDateRangeEnquirer dateRangeEnquirer)
    {
        _musicVideoList = musicVideoList;
        _dateRangeEnquirer = dateRangeEnquirer;
        _categoryFilters = new CategoryFilters();
        _dateFilters = new DateFilters();
        ResetFilters();
    }

    public bool TryUpdateFilter(string action)
    {
        switch (action)
        {
            case VideoListActions.ToggleMusicVideo:
            case VideoListActions.ToggleMusicVideoOnly:
            case VideoListActions.ToggleBandLive:
            case VideoListActions.ToggleBandLiveOnly:
            case VideoListActions.TogglePerformance:
            case VideoListActions.TogglePerformanceOnly:
            case VideoListActions.ToggleChoreography:
            case VideoListActions.ToggleChoreographyOnly:
            case VideoListActions.ToggleRelay:
            case VideoListActions.ToggleRelayOnly:
            case VideoListActions.ToggleBeOriginal:
            case VideoListActions.ToggleBeOriginalOnly:
            case VideoListActions.ToggleFancam:
            case VideoListActions.ToggleFancamOnly:
            case VideoListActions.ToggleConcert:
            case VideoListActions.ToggleConcertOnly:
            case VideoListActions.ToggleMusicShow:
            case VideoListActions.ToggleMusicShowOnly:
                var actionResult = ParsedCategoryFilterAction.ReadAction(action) ??
                                   throw new InvalidOperationException($"Action {action} not found");
                _categoryFilters.Update(actionResult);
                return true;
            case VideoListActions.ReleaseDateFilter:
                var releaseDateInput =
                    _dateRangeEnquirer.AskDateRangeOrSingleDate("Enter date or range to filter tracks by");
                if (_dateFilters.TryUpdateTrackReleaseDate(releaseDateInput))
                {
                    return true;
                }

                break;

            case VideoListActions.VideoDateFilter:
                var videoDateInput =
                    _dateRangeEnquirer.AskDateRangeOrSingleDate("Enter date or range to filter tracks by");
                if (_dateFilters.TryUpdateVideoReleaseDate(videoDateInput))
                {
                    return true;
                }

                break;

            case VideoListActions.Clear:
                ResetFilters();
                return true;
            default:
                return false;
        }

        return false;
    }


    public List<string> ToFiltered()
    {
        var videos = _musicVideoList.VideoList();
        var musicVideos = videos.Select(_musicVideoList.MusicVideoFor).ToList();
        var categoryFilteredVideos = _categoryFilters.ToFiltered(musicVideos);
        var dateFilteredVideos = _dateFilters.ToFiltered(categoryFilteredVideos);
        return dateFilteredVideos.Select(video => Path.GetFileName(video.FilePath)).ToList();
    }

    private void ResetFilters()
    {
        _dateFilters.Reset();
        _categoryFilters.MusicVideosOnly();
    }
}