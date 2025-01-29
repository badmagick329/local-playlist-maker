namespace PlaylistMaker.Core;

public interface IMusicVideoList
{
    string VideoPathFor(string videoName);
    string AudioPathFor(string videoName);
    List<string> ReadAllPaths();

    MusicVideo MusicVideoFor(string videoName);
    List<string> VideoList();
    IEnumerable<(string artist, int count)> TopArtists(int i);
}
