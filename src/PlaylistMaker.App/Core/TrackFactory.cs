namespace PlaylistMaker.Core;

public static class TrackFactory
{
    public static Track? FromVorbisData(VorbisData vorbisData)
    {
        var releaseDate = ReleaseDateFactory.TryFromTagOrPath(vorbisData.Date, vorbisData.FilePath);

        return releaseDate is null
            ? null
            : new Track(vorbisData.TrackNumber, vorbisData.Artist,
                vorbisData.Title, releaseDate, vorbisData.FilePath);
    }

    public static List<Track>
        FromVorbisDataList(List<VorbisData> vorbisDatas) =>
        vorbisDatas
            .Select(FromVorbisData)
            .Where(t => t is not null)
            .ToList()!;
}
