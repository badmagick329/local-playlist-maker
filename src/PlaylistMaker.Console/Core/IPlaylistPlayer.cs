namespace PlaylistMaker.Core;

public interface IPlaylistPlayer
{
    void Play(string playlistPath);
    void CreateAndPlay(List<string> trackPaths);
}