using PlaylistMaker.Core;

namespace PlaylistMaker.Infrastructure;

public class PlaylistTxtFileReader : IPlaylistTxtFileReader
{
    private string FilePath { get; }

    public PlaylistTxtFileReader(string filePath)
    {
        FilePath = filePath;
    }

    public List<string> Read()
    {
        if (!File.Exists(FilePath))
        {
            throw new FileNotFoundException($"Playlist file not found: {FilePath}");
        }

        return File.ReadAllLines(FilePath)
            .Select(line => Path.GetFileName(line.Trim()))
            .Where(line => !string.IsNullOrEmpty(line) && !line.StartsWith("#"))
            .ToList();
    }
}
