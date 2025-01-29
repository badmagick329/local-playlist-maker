using PlaylistMaker.Core.VideoFilters;
using PlaylistMaker.Infrastructure;

namespace PlaylistMaker.View;

public static class ParsedCategoryFilterAction
{
    public static (string category, ToggleType type)? ReadAction(string action) =>
        action switch
        {
            VideoListActions.ToggleMusicVideo => (CategoryFilterNames.MusicVideo, ToggleType.Show),
            VideoListActions.ToggleMusicVideoOnly => (CategoryFilterNames.MusicVideo, ToggleType.Only),
            VideoListActions.ToggleBandLive => (CategoryFilterNames.BandLive, ToggleType.Show),
            VideoListActions.ToggleBandLiveOnly => (CategoryFilterNames.BandLive, ToggleType.Only),
            VideoListActions.TogglePerformance => (CategoryFilterNames.Performance, ToggleType.Show),
            VideoListActions.TogglePerformanceOnly => (CategoryFilterNames.Performance, ToggleType.Only),
            VideoListActions.ToggleChoreography => (CategoryFilterNames.Choreography, ToggleType.Show),
            VideoListActions.ToggleChoreographyOnly => (CategoryFilterNames.Choreography, ToggleType.Only),
            VideoListActions.ToggleRelay => (CategoryFilterNames.Relay, ToggleType.Show),
            VideoListActions.ToggleRelayOnly => (CategoryFilterNames.Relay, ToggleType.Only),
            VideoListActions.ToggleBeOriginal => (CategoryFilterNames.BeOriginal, ToggleType.Show),
            VideoListActions.ToggleBeOriginalOnly => (CategoryFilterNames.BeOriginal, ToggleType.Only),
            VideoListActions.ToggleFancam => (CategoryFilterNames.Fancam, ToggleType.Show),
            VideoListActions.ToggleFancamOnly => (CategoryFilterNames.Fancam, ToggleType.Only),
            VideoListActions.ToggleConcert => (CategoryFilterNames.Concert, ToggleType.Show),
            VideoListActions.ToggleConcertOnly => (CategoryFilterNames.Concert, ToggleType.Only),
            VideoListActions.ToggleMusicShow => (CategoryFilterNames.MusicShow, ToggleType.Show),
            VideoListActions.ToggleMusicShowOnly => (CategoryFilterNames.MusicShow, ToggleType.Only),
            _ => null
        };
}