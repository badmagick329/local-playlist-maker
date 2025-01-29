using PlaylistMaker.Core;

namespace PlaylistMaker.Tests.Mocks;

public class MockMusicVideoList : IMusicVideoList
{
    private readonly Dictionary<string, string> _videoNameToPath;

    public MockMusicVideoList() =>
        _videoNameToPath = MockVideoPaths.CreateMap(MockVideoPaths.FullList());

    public MockMusicVideoList(IEnumerable<string> videoPaths) =>
        _videoNameToPath = MockVideoPaths.CreateMap(videoPaths);

    public string VideoPathFor(string videoName) => "";


    public string AudioPathFor(string videoName) => "";

    public List<string> ReadAllPaths() => [];

    public MusicVideo MusicVideoFor(string videoName)
    {
        var videoPath = _videoNameToPath[videoName];
        var track = MockTrackFactory.FromVideoPath(videoPath);
        return new MusicVideo(videoPath, track);
    }

    public List<string> VideoList() => _videoNameToPath.Keys.ToList();

    public IEnumerable<(string artist, int count)> TopArtists(int i)
    {
        throw new NotImplementedException();
    }
}

static class MockVideoPaths
{
    public static Dictionary<string, string> CreateMap(IEnumerable<string> videoPaths)
    {
        return videoPaths.ToDictionary(
            videoPath => Path.GetFileName(videoPath) ?? throw new ArgumentException("Invalid path"),
            videoPath => videoPath
        );
    }

    public static IEnumerable<string> FullList() =>
        MusicVideo()
            .Concat(Performance())
            .Concat(Choreography())
            .Concat(Relay())
            .Concat(Fancam())
            .Concat(Concert())
            .Concat(BandLive())
            .Concat(HdLive())
            .Concat(UhdLive());


    public static IEnumerable<string> MusicVideo() => [@"F:\Music\MVs\190918 Dreamcatcher - Deja Vu.mkv"];
    public static IEnumerable<string> Performance() => [@"F:\Music\MVs\230207 VIVIZ - Pull Up Performance.mkv"];
    public static IEnumerable<string> Choreography() => [@"F:\Music\MVs\220916 BLACKPINK - Shut Down Choreography.mkv"];

    public static IEnumerable<string> Relay() =>
        [@"F:\Music\MVs\230529 LE SSERAFIM - Eve, Psych and Bluebeard's Wife Relay.mkv"];

    public static IEnumerable<string> BeOriginal() => [@"F:\Music\MVs\230412 IVE - I am Be Original.mkv"];

    public static IEnumerable<string> Fancam() =>
    [
        @"F:\Music\MVs\211007 Twice - The Feels (Jihyo Fancam).mkv",
        @"F:\Music\MVs\220502 LE SSERAFIM - FEARLESS (Fancam).mkv",
    ];

    public static IEnumerable<string> Concert() =>
    [
        @"F:\Music\MVs\190525 Twice - 23 Ho! (Concert).mkv",
        @"F:\Music\MVs\130209 Girls Generation - 09 T.O.P (Japan Concert).mkv",
        @"F:\Music\MVs\180904 Red Velvet - Power Up (Seoul Drama Awards Concert).mp4",
    ];

    public static IEnumerable<string> BandLive() => [@"F:\Music\MVs\230520 Aespa - Thirsty (Band Live).webm"];

    public static IEnumerable<string> HdLive() =>
        [@"D:\Music\uhdkpop\121113 Girls Generation - Flower Power (HD Live).mp4"];

    public static IEnumerable<string> UhdLive() => [@"D:\Music\uhdkpop\201226 IZONE - Sequence (UHD Live).mkv"];
}