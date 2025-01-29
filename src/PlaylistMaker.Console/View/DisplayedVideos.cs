using PlaylistMaker.Application;
using PlaylistMaker.Core;
using PlaylistMaker.Infrastructure;

namespace PlaylistMaker.View;

public class DisplayedVideos
{
    public List<string> Videos { get; private set; }

    private readonly VideoSorter _videoSorter;
    private readonly VideoFilterer _videoFilterer;

    public DisplayedVideos(IMusicVideoList musicVideoList, IDateRangeEnquirer dateRangeEnquirer)
    {
        _videoSorter = new VideoSorter(musicVideoList);
        _videoFilterer = new VideoFilterer(musicVideoList, dateRangeEnquirer);
        Videos = _videoSorter.ToSorted(_videoFilterer.ToFiltered());
    }

    public static bool IsExiting(List<string> choices) => choices is [VideoListActions.Back] or [VideoListActions.Quit];

    public bool TryUpdateState(List<string> choices)
    {
        if (choices.Count != 1)
        {
            return false;
        }

        var filterUpdated = _videoFilterer.TryUpdateFilter(choices[0]);
        if (filterUpdated)
        {
            var filteredVideos = _videoFilterer.ToFiltered();
            Videos = _videoSorter.ToSorted(filteredVideos);
            return true;
        }

        var sortUpdated = _videoSorter.TryUpdateSorting(choices[0]);
        // ReSharper disable once InvertIf
        if (sortUpdated)
        {
            Videos = _videoSorter.ToSorted(Videos);
            return true;
        }

        return false;
    }

    public void SetWithoutActions(List<string> choices) =>
        Videos = VideoListActions.RemoveActionsFromList(choices);
}