using YamlDotNet.Serialization;
using YamlDotNet.Serialization.NamingConventions;

namespace PlaylistMaker.Core;

public class Config
{
    public string DataDirectory { get; set; }
    public List<string> MusicVideoToAudioMap { get; set; }
    public string FlacsMegaPlaylist { get; set; }
    public string FlacCacheFile { get; set; }
    public string PlaylistTemplate { get; set; }
    public string[] VideoPlaylistCommand { get; set; }
    public string VideoPlaylistSuffix { get; set; }
    public string[] AudioPlaylistCommand { get; set; }
    public string AudioPlaylistSuffix { get; set; }
    public string[] VideoSingleFileCommand { get; set; }
    public string[] AudioSingleFileCommand { get; set; }
    public string PlaylistTxtFilePath { get; set; }
}

public class ConfigReader(string configPath)
{
    public string ConfigPath { get; init; } = configPath;

    public Config ReadConfig()
    {
        IDeserializer deserializer = new DeserializerBuilder()
            .WithNamingConvention(CamelCaseNamingConvention.Instance)
            .Build();

        string rawYaml = File.ReadAllText(ConfigPath);
        Config config = deserializer.Deserialize<Config>(rawYaml);
        return config;
    }
}
