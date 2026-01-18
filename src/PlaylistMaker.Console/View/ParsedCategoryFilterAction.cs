using PlaylistMaker.Core.VideoFilters;
using PlaylistMaker.Infrastructure;

namespace PlaylistMaker.View;

public static class ParsedCategoryFilterAction
{
    public static (string category, ToggleType type)? ReadAction(string action) =>
        action switch
        {
            VideoListActions.ToggleMusicVideo => (CategoryFilterNames.MusicVideo, ToggleType.Show),
            VideoListActions.ToggleBandLive => (CategoryFilterNames.BandLive, ToggleType.Show),
            VideoListActions.TogglePerformance => (
                CategoryFilterNames.Performance,
                ToggleType.Show
            ),
            VideoListActions.ToggleChoreography => (
                CategoryFilterNames.Choreography,
                ToggleType.Show
            ),
            VideoListActions.ToggleRelay => (CategoryFilterNames.Relay, ToggleType.Show),
            VideoListActions.ToggleBeOriginal => (CategoryFilterNames.BeOriginal, ToggleType.Show),
            VideoListActions.ToggleFancam => (CategoryFilterNames.Fancam, ToggleType.Show),
            VideoListActions.ToggleConcert => (CategoryFilterNames.Concert, ToggleType.Show),
            VideoListActions.ToggleMusicShow => (CategoryFilterNames.MusicShow, ToggleType.Show),
            VideoListActions.ToggleLiveAudio => (CategoryFilterNames.LiveAudio, ToggleType.Show),
            _ => null,
        };
}
