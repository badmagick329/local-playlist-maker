namespace PlaylistMaker.Core;

public interface IVideoToAudioMapReader
{
    Dictionary<string, string> ReadMapper();
}
