namespace PlaylistMaker.Core;

public record VorbisData(
    string FilePath,
    string Artist,
    string Title,
    string Date,
    int TrackNumber,
    string LastRead);
