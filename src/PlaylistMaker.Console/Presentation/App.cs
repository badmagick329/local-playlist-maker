using PlaylistMaker.Application;
using PlaylistMaker.Core;
using PlaylistMaker.Infrastructure;

namespace PlaylistMaker.Presentation;

public class App
{
    private readonly IVorbisReader _vorbisReader;
    private readonly IImportedVideoToAudioMap _importedVideoToAudioMap;
    private readonly IUserInputReader _userInputReader;
    private readonly IPlaylistTxtFileReader _playlistTxtFileReader;
    private readonly IPlaylistPlayer _flacPlaylistPlayer;
    private readonly IPlaylistPlayer _videoPlaylistPlayer;
    private readonly PlaybackHistoryService? _playbackHistory;

    public App(
        IVorbisReader reader,
        IImportedVideoToAudioMap importedVideoToAudioMap,
        IUserInputReader userInputReader,
        IPlaylistTxtFileReader playlistTxtFileReader,
        IPlaylistPlayer flacPlaylistPlayer,
        IPlaylistPlayer videoPlaylistPlayer,
        PlaybackHistoryService? playbackHistory = null
    )
    {
        _vorbisReader = reader;
        _importedVideoToAudioMap = importedVideoToAudioMap;
        _userInputReader = userInputReader;
        _playlistTxtFileReader = playlistTxtFileReader;
        _flacPlaylistPlayer = flacPlaylistPlayer;
        _videoPlaylistPlayer = videoPlaylistPlayer;
        _playbackHistory = playbackHistory;
    }

    public void Run()
    {
        RunVideoPlaylistApp();
    }

    private void RunFlacPlaylistApp()
    {
        var app = new FlacPlaylistApp(_vorbisReader, _userInputReader, _flacPlaylistPlayer);
        app.Run();
    }

    private void RunVideoPlaylistApp()
    {
        var fzfSelector = new FzfSelector();
        var musicVideoList = new MusicVideoList(_vorbisReader, _importedVideoToAudioMap.Import());
        var missingPaths = musicVideoList.ReadVideoPath().Where(p => !File.Exists(p)).ToList();
        if (missingPaths.Count > 0)
        {
            Console.WriteLine("Missing paths:");
            foreach (var path in missingPaths)
            {
                Console.WriteLine(path);
            }

            return;
        }

        var view = new VideoPlaylistController(
            fzfSelector,
            musicVideoList,
            _userInputReader,
            _playlistTxtFileReader
        );
        view.AskForVideosAndAudios(PlayVideosAndAudios);
    }

    private void PlayVideosAndAudios(PlaybackSelection selection)
    {
        var videos = selection.Videos.Select(video => video.FilePath).ToList();
        var audios = selection.Videos.Select(video => video.Track.FilePath).ToList();
        var historySession = _playbackHistory?.CreateSession(selection.Videos, selection.Source);

        _flacPlaylistPlayer.CreateAndPlay(audios);
        var mpvProcessId = _videoPlaylistPlayer.CreateAndPlay(
            videos,
            historySession is null ? null : _playbackHistory!.MpvArgumentsFor(historySession)
        );

        if (historySession is not null && mpvProcessId is not null)
        {
            _playbackHistory!.RecordMpvProcess(historySession, mpvProcessId.Value);
        }
    }
}
