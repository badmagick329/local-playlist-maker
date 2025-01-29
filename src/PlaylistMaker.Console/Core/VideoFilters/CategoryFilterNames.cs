namespace PlaylistMaker.Core.VideoFilters;

public static class CategoryFilterNames
{
    public const string
        MusicVideo = "Music Video",
        BandLive = "Band Live",
        Performance = "Performance",
        Choreography = "Choreography",
        Relay = "Relay",
        BeOriginal = "Be Original",
        Fancam = "Fancam",
        Concert = "Concert",
        MusicShow = "Music Show";

    public static List<string> AllCategories() =>
    [
        MusicVideo,
        BandLive,
        Performance,
        Choreography,
        Relay,
        BeOriginal,
        Fancam,
        Concert,
        MusicShow
    ];
}