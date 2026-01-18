using PlaylistMaker.Application;
using PlaylistMaker.Core;
using PlaylistMaker.Infrastructure;
using PlaylistMaker.View;

namespace PlaylistMaker.Presentation;

public class VideoPlaylistController
{
    private readonly IChoicesSelecter _choicesSelecter;
    private readonly IMusicVideoList _musicVideoList;
    private readonly DisplayedVideosAndActions _displayedVideos;
    private readonly PlaybackPreProcessor _playbackPreProcessor;
    private readonly IPlaylistTxtFileReader _playlistTxtFileReader;

    public VideoPlaylistController(
        IChoicesSelecter choicesSelecter,
        IMusicVideoList musicVideoList,
        IUserInputReader userInputReader,
        IPlaylistTxtFileReader playlistTxtFileReader
    )
    {
        _choicesSelecter = choicesSelecter;
        _musicVideoList = musicVideoList;
        _displayedVideos = new DisplayedVideosAndActions(_musicVideoList, userInputReader);
        _playbackPreProcessor = new PlaybackPreProcessor(_musicVideoList, userInputReader);
        _playlistTxtFileReader = playlistTxtFileReader;
    }

    public void AskForVideosAndAudios(
        Action<(List<string> videos, List<string> audios)> onVideosAndAudiosSelected
    )
    {
        while (true)
        {
            var videosWithActions = VideoListActions.AddActionsToList(_displayedVideos.Videos);
            var choices = _choicesSelecter.AskStringsContainedIn(videosWithActions);

            if (DisplayedVideosAndActions.IsExiting(choices))
            {
                return;
            }

            if (TryProcessActions(choices))
            {
                continue;
            }

            var playbackVideos = GetPlaybackVideos(choices);

            onVideosAndAudiosSelected(
                (
                    videos: playbackVideos.Select(c => _musicVideoList.VideoPathFor(c)).ToList(),
                    audios: playbackVideos.Select(c => _musicVideoList.AudioPathFor(c)).ToList()
                )
            );
        }
    }

    private bool TryProcessActions(List<string> choices) =>
        _playbackPreProcessor.TryUpdatePlayMethod(choices)
        || _displayedVideos.TryUpdateState(choices);

    private List<string> GetPlaybackVideos(List<string> choices)
    {
        List<string> selectedVideos;

        if (DisplayedVideosAndActions.IsSelectFromTxtFile(choices))
        {
            selectedVideos = _playlistTxtFileReader.Read();
        }
        else
        {
            selectedVideos = DisplayedVideosAndActions.IsInvertedSelection(choices)
                ? _displayedVideos.Videos
                : DisplayedVideosAndActions.GetWithoutActions(choices);
        }

        return _playbackPreProcessor.Process(selectedVideos);
    }
}
