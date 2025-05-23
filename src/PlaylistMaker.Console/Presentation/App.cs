﻿using PlaylistMaker.Application;
using PlaylistMaker.Core;
using PlaylistMaker.Infrastructure;

namespace PlaylistMaker.Presentation;

public class App
{
    private readonly IVorbisReader _vorbisReader;
    private readonly IImportedVideoToAudioMap _importedVideoToAudioMap;
    private readonly IUserInputReader _userInputReader;
    private readonly IPlaylistPlayer _flacPlaylistPlayer;
    private readonly IPlaylistPlayer _videoPlaylistPlayer;

    public App(
        IVorbisReader reader,
        IImportedVideoToAudioMap importedVideoToAudioMap,
        IUserInputReader userInputReader,
        IPlaylistPlayer flacPlaylistPlayer,
        IPlaylistPlayer videoPlaylistPlayer
    )
    {
        _vorbisReader = reader;
        _importedVideoToAudioMap = importedVideoToAudioMap;
        _userInputReader = userInputReader;
        _flacPlaylistPlayer = flacPlaylistPlayer;
        _videoPlaylistPlayer = videoPlaylistPlayer;
    }

    public void Run()
    {
        while (true)
        {
            Console.WriteLine("Choose an option:");
            Console.WriteLine("1. Flac Playlist");
            Console.WriteLine("2. Video Playlist");
            Console.WriteLine("3. Quit");
            var option = _userInputReader.AskInt("Enter option");

            switch (option)
            {
                case 1:
                    RunFlacPlaylistApp();
                    return;
                case 2:
                    RunVideoPlaylistApp();
                    return;
                case 3:
                    Environment.Exit(0);
                    break;
                default:
                    Console.WriteLine("Invalid option");
                    break;
            }
        }
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

        var view = new VideoPlaylistController(fzfSelector, musicVideoList, _userInputReader);
        view.AskForVideosAndAudios(PlayVideosAndAudios);
    }

    private void PlayVideosAndAudios((List<string> videos, List<string> audios) videosAndAudios)
    {
        var (videos, audios) = videosAndAudios;
        _flacPlaylistPlayer.CreateAndPlay(audios);
        _videoPlaylistPlayer.CreateAndPlay(videos);
    }
}
