using PlaylistMaker.Core;

namespace PlaylistMaker.Infrastructure;

public class PlaylistPlayerConfig
{
    public ICliCommand PlaylistCommand { get; set; }
    public ICliCommand? SingleFileCommand { get; set; }
    private string _playlistArgumentTemplate;

    public string PlaylistArgumentTemplate
    {
        get => _playlistArgumentTemplate;
        set
        {
            if (string.IsNullOrWhiteSpace(value))
            {
                throw new ArgumentException("Playlist argument template cannot be null or empty");
            }

            _playlistArgumentTemplate = value;
        }
    }

    private string _playlistDirectory;

    public string PlaylistDirectory
    {
        get => _playlistDirectory;
        set
        {
            if (!Directory.Exists(value))
            {
                throw new ArgumentException("Directory does not exist");
            }

            _playlistDirectory = value;
        }
    }

    private string _playlistSuffix;

    public string PlaylistSuffix
    {
        get => _playlistSuffix;
        set
        {
            if (string.IsNullOrWhiteSpace(value))
            {
                throw new ArgumentException("Playlist suffix cannot be null or empty");
            }

            _playlistSuffix = value;
        }
    }

    public override string ToString()
    {
        return $"PlaylistCommand: {PlaylistCommand}, SingleFileCommand: {SingleFileCommand}, " +
               $"PlaylistArgumentTemplate: {PlaylistArgumentTemplate}, PlaylistDirectory: {PlaylistDirectory}, " +
               $"PlaylistSuffix: {PlaylistSuffix}";
    }
}