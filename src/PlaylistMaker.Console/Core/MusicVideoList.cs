using PlaylistMaker.Exceptions;

namespace PlaylistMaker.Core;

public class MusicVideoList : IMusicVideoList
{
    private Dictionary<string, MusicVideo> Videos { get; set; } = [];

    public MusicVideoList(IVorbisReader reader, Dictionary<string, string> mapper)
    {
        foreach (var (videoPath, audioPath) in mapper)
        {
            var vorbisData = reader.VorbisDataFor(audioPath) ??
                             throw new VideoPlayerException($"No vorbis data found for {audioPath}");
            var track = TrackFactory.FromVorbisData(vorbisData) ??
                        throw new VideoPlayerException($"Track is null. {audioPath}");
            var musicVideo = new MusicVideo(videoPath, track);
            Videos.Add(Path.GetFileName(videoPath), musicVideo);
        }
    }

    public string VideoPathFor(string videoName) => MusicVideoFor(videoName).FilePath;

    public string AudioPathFor(string videoName) => MusicVideoFor(videoName).Track.FilePath;

    public MusicVideo MusicVideoFor(string videoName) =>
        Videos.TryGetValue(videoName, out MusicVideo? value)
            ? value
            : throw new VideoPlayerException($"No music video found for {videoName}");

    public List<string> VideoList() => Videos.Keys.ToList();

    public List<string> ReadAllPaths() =>
        Videos.Values
            .SelectMany(v => new[] { v.FilePath, v.Track.FilePath })
            .Distinct()
            .ToList();

    public IEnumerable<(string artist, int count)> TopArtists(int i)
    {
        return Videos.Values
            .GroupBy(v => v.Track.Artist)
            .Select(g => (g.Key, g.Count()))
            .OrderByDescending(t => t.Item2)
            .Take(i);
    }
}
