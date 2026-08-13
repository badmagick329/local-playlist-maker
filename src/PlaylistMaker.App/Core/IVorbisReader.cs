namespace PlaylistMaker.Core;

public interface IVorbisReader
{
    VorbisData? VorbisDataFor(string filePath);
    List<string> GetAllFilePaths();
}
