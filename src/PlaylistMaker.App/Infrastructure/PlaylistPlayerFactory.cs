using PlaylistMaker.Core;

namespace PlaylistMaker.Infrastructure;

public static class PlaylistPlayerFactory
{
    public static PlaylistPlayer CreateVideo(Config config, bool isolateChildProcessIO = false) => new(new PlaylistPlayerConfig
    {
        PlaylistCommand = CliCommand.CreateFromList(config.VideoPlaylistCommand),
        SingleFileCommand = CliCommand.CreateFromList(config.VideoSingleFileCommand),
        PlaylistDirectory = config.DataDirectory,
        PlaylistSuffix = config.VideoPlaylistSuffix,
        PlaylistArgumentTemplate = config.PlaylistTemplate,
    }, isolateChildProcessIO);

    public static PlaylistPlayer CreateAudio(Config config, bool isolateChildProcessIO = false) => new(new PlaylistPlayerConfig
    {
        PlaylistCommand = CliCommand.CreateFromList(config.AudioPlaylistCommand),
        SingleFileCommand = CliCommand.CreateFromList(config.AudioSingleFileCommand),
        PlaylistDirectory = config.DataDirectory,
        PlaylistSuffix = config.AudioPlaylistSuffix,
        PlaylistArgumentTemplate = config.PlaylistTemplate,
    }, isolateChildProcessIO);
}
