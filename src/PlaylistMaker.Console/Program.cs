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
        Config = new ConfigReader(ConfigPath).ReadConfig();
        RunPlaylistMaker();
    }

    private static void RunPlaylistMaker()
    {
        var flacPathsReader =
            new FlacPathsReader(
                Config.FlacsMegaPlaylist, Config.MusicVideoToAudioMap);
        var vorbisReader =
            new VorbisReader(flacPathsReader, Config.FlacCacheFile);
        var videoToAudioMapReader =
            new ImportedVideoToAudioMap([
                Config.MusicVideoToAudioMap, Config.MusicShowVideoToAudioMap
            ]);
        var app = new App(
            vorbisReader,
            videoToAudioMapReader,
            new UserInputReader(),
            new FlacPlaylistPlayer(),
            new VideoPlaylistPlayer()
        );
        app.Run();
    }
}