using System.Text.Json;
using PlaylistMaker.Common;
using PlaylistMaker.Core;

namespace PlaylistMaker.Infrastructure;

public class VideoToAudioMapReader : IVideoToAudioMapReader
{
    private readonly List<string> _fileNames;

    public VideoToAudioMapReader(List<string> fileNames)
    {
        _fileNames = fileNames;
    }

    public Dictionary<string, string> ReadMapper()
    {
        var jsonData = new Dictionary<string, string>();
        foreach (var fileName in _fileNames)
        {
            var fileData = JsonSerializer.Deserialize<Dictionary<string, string>>(
                File.ReadAllText(fileName), new JsonSerializerOptions
                {
                    PropertyNameCaseInsensitive = true
                }
            ) ?? throw new Exception("Could not deserialize");
            foreach (var (key, value) in fileData)
            {
                jsonData[key] = PathOperations.FixFlacPath(value);
            }
        }

        return jsonData;
    }
}