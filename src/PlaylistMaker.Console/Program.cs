using PlaylistMaker.Core;
using PlaylistMaker.Infrastructure;
using PlaylistMaker.Presentation;

// ReSharper disable once CheckNamespace
static class Program
{
    public const string ConfigPath = "config.yaml";
    private static Config Config { get; set; }

    public static void Main(string[] args)
    {
        if (!File.Exists(ConfigPath))
        {
            Console.WriteLine($"{ConfigPath} not found");
            return;
        }

        try
        {
            Config = new ConfigReader(ConfigPath).ReadConfig();
        }
        catch (Exception e)
        {
            Console.WriteLine($"Error reading config: {e.Message}");
            return;
        }

        RunPlaylistMaker();
    }

    private static void RunPlaylistMaker()
    {
        var flacPathsReader =
            new FlacPathsReader(
                Config.FlacsMegaPlaylist, Config.MusicVideoToAudioMap);
        var vorbisReader =
            new VorbisReader(flacPathsReader, Path.Combine(Config.DataDirectory, Config.FlacCacheFile));
        var videoToAudioMapReader =
            new ImportedVideoToAudioMap([
                Config.MusicVideoToAudioMap, Config.MusicShowVideoToAudioMap
            ]);
        var app = new App(
            vorbisReader,
            videoToAudioMapReader,
            new UserInputReader(),
            CreateAudioPlaylistPlayer(),
            CreateVideoPlaylistPlayer()
        );
        app.Run();
    }

    private static PlaylistPlayer CreateVideoPlaylistPlayer()
    {
        var videoPlaylistCommand = CliCommand.CreateFromList(Config.VideoPlaylistCommand);
        var videoSingleFileCommand = CliCommand.CreateFromList(Config.VideoSingleFileCommand);
        var videoPlaylistPlayerConfig = new PlaylistPlayerConfig
        {
            PlaylistCommand = videoPlaylistCommand,
            SingleFileCommand = videoSingleFileCommand,
            PlaylistDirectory = Config.DataDirectory,
            PlaylistSuffix = Config.VideoPlaylistSuffix,
            PlaylistArgumentTemplate = Config.PlaylistTemplate
        };
        return new PlaylistPlayer(videoPlaylistPlayerConfig);
    }

    private static PlaylistPlayer CreateAudioPlaylistPlayer()
    {
        var audioPlaylistCommand = CliCommand.CreateFromList(Config.AudioPlaylistCommand);
        var audioSingleFileCommand = CliCommand.CreateFromList(Config.AudioSingleFileCommand);
        var audioPlaylistPlayerConfig = new PlaylistPlayerConfig
        {
            PlaylistCommand = audioPlaylistCommand,
            SingleFileCommand = audioSingleFileCommand,
            PlaylistDirectory = Config.DataDirectory,
            PlaylistSuffix = Config.AudioPlaylistSuffix,
            PlaylistArgumentTemplate = Config.PlaylistTemplate
        };
        return new PlaylistPlayer(audioPlaylistPlayerConfig);
    }
}