namespace PlaylistMaker.Core;

public interface IPlaylistPlayer
{
    void Play(string playlistPath);
    int? CreateAndPlay(List<string> trackPaths, IReadOnlyList<string>? additionalArguments = null);
}
