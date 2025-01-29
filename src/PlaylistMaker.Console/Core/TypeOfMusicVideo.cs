using System.Text.RegularExpressions;

namespace PlaylistMaker.Core;

public static partial class TypeOfMusicVideo
{
    public static bool IsBandLive(MusicVideo musicVideo) =>
        BandLiveRegex().Match(Path.GetFileNameWithoutExtension(musicVideo.FilePath)).Success;

    public static bool IsPerformance(MusicVideo musicVideo) =>
        PerformanceRegex().Match(Path.GetFileNameWithoutExtension(musicVideo.FilePath)).Success;

    public static bool IsChoreography(MusicVideo musicVideo) =>
        ChoreographyRegex().Match(Path.GetFileNameWithoutExtension(musicVideo.FilePath)).Success;

    public static bool IsRelay(MusicVideo musicVideo) =>
        RelayRegex().Match(Path.GetFileNameWithoutExtension(musicVideo.FilePath)).Success;

    public static bool IsBeOriginal(MusicVideo musicVideo) =>
        BeOriginalRegex().Match(Path.GetFileNameWithoutExtension(musicVideo.FilePath)).Success;

    public static bool IsFancam(MusicVideo musicVideo) =>
        FancamRegex().Match(Path.GetFileNameWithoutExtension(musicVideo.FilePath)).Success;

    public static bool IsConcert(MusicVideo musicVideo) =>
        ConcertRegex().Match(Path.GetFileNameWithoutExtension(musicVideo.FilePath)).Success;

    public static bool IsMusicVideo(MusicVideo musicVideo) =>
        !IsBandLive(musicVideo) && !IsPerformance(musicVideo) && !IsChoreography(musicVideo) && !IsRelay(musicVideo) &&
        !IsBeOriginal(musicVideo) && !IsFancam(musicVideo) && !IsConcert(musicVideo) && !IsMusicShow(musicVideo);

    public static bool IsMusicShow(MusicVideo musicVideo) => musicVideo.FilePath.Contains(@"Music\uhdkpop");


    [GeneratedRegex(@"(?i).+\s+-\s+.+\(.*band live.*\)$")]
    private static partial Regex BandLiveRegex();

    [GeneratedRegex(@"(?i).+\s+-\s+.+\sperformance$")]
    private static partial Regex PerformanceRegex();

    [GeneratedRegex(@"(?i).+\s+-\s+.+\schoreography$")]
    private static partial Regex ChoreographyRegex();

    [GeneratedRegex(@"(?i).+\s+-\s+.+\srelay$")]
    private static partial Regex RelayRegex();

    [GeneratedRegex(@"(?i).+\s+-\s+.+\sbe original$")]
    private static partial Regex BeOriginalRegex();

    [GeneratedRegex(@"(?i).+\s+-\s+.+\(.*fancam.*\)$")]
    private static partial Regex FancamRegex();

    [GeneratedRegex(@"(?i).+\s+-\s+.+\(.*concert.*\)$")]
    private static partial Regex ConcertRegex();
}
