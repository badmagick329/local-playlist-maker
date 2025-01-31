using PlaylistMaker.Application;
using PlaylistMaker.Core;
using PlaylistMaker.Infrastructure;
using PlaylistMaker.View;

namespace PlaylistMaker.Presentation;

public class VideoPlaylistController
{
    private readonly IChoicesSelecter _choicesSelecter;
    private readonly IMusicVideoList _musicVideoList;
    private readonly DisplayedVideos _displayedVideos;
    private readonly PlaybackPreProcessor _playbackPreProcessor;

    public VideoPlaylistController(IChoicesSelecter choicesSelecter, IMusicVideoList musicVideoList,
        IUserInputReader userInputReader)
    {
        _choicesSelecter = choicesSelecter;
        _musicVideoList = musicVideoList;
        _displayedVideos = new DisplayedVideos(_musicVideoList, userInputReader);
        _playbackPreProcessor = new PlaybackPreProcessor(_musicVideoList, userInputReader);
    }

    public void AskForVideosAndAudios(Action<(List<string> videos, List<string> audios)> callback)
    {
        while (true)
        {
            var videosWithActions = VideoListActions.AddActionsToList(_displayedVideos.Videos);
            var choices = _choicesSelecter.AskStringsContainedIn(videosWithActions);

            if (DisplayedVideos.IsExiting(choices))
            {
                return;
            }

            if (_playbackPreProcessor.TryUpdatePlayMethod(choices))
            {
                continue;
            }

            if (_displayedVideos.TryUpdateState(choices))
            {
                continue;
            }

            var selectedVideos = DisplayedVideos.IsInvertedSelection(choices)
                ? _displayedVideos.Videos
                : DisplayedVideos.GetWithoutActions(choices);

            var playbackVideos = _playbackPreProcessor.Process(selectedVideos);
            callback((videos: playbackVideos.Select(c => _musicVideoList.VideoPathFor(c)).ToList(),
                audios: playbackVideos.Select(c => _musicVideoList.AudioPathFor(c)).ToList()));
        }
    }
}