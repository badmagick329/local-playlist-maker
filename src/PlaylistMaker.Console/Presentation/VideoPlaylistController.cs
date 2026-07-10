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
        Action<PlaybackSelection> onVideosAndAudiosSelected
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

            onVideosAndAudiosSelected(GetPlaybackSelection(choices));
        }
    }

    private bool TryProcessActions(List<string> choices) =>
        _playbackPreProcessor.TryUpdatePlayMethod(choices)
        || _displayedVideos.TryUpdateState(choices);

    private PlaybackSelection GetPlaybackSelection(List<string> choices)
    {
        List<string> selectedVideos;
        string source;

        if (DisplayedVideosAndActions.IsSelectFromTxtFile(choices))
        {
            selectedVideos = _playlistTxtFileReader.Read();
            source = "text-playlist";
        }
        else
        {
            selectedVideos = DisplayedVideosAndActions.IsInvertedSelection(choices)
                ? _displayedVideos.Videos
                : DisplayedVideosAndActions.GetWithoutActions(choices);
            source = "interactive-selection";
        }

        var playbackVideos = _playbackPreProcessor.Process(selectedVideos);
        return new PlaybackSelection(
            playbackVideos.Select(_musicVideoList.MusicVideoFor).ToList(),
            source
        );
    }
}
