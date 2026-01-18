namespace PlaylistMaker.Infrastructure;

public static class VideoListActions
{
    public const string Back = "[[Back]]",
        Quit = "[[Quit]]",
        OrderByName = "[[Order By Name Ascending]]",
        OrderByNameDesc = "[[Order By Name Descending]]",
        OrderByReleaseDate = "[[Order By Release Date Ascending]]",
        OrderByReleaseDateDesc = "[[Order By Release Date Descending]]",
        OrderByMTime = "[[Order By MTime Ascending]]",
        OrderByMTimeDesc = "[[Order By MTime Descending]]",
        OrderByArtist = "[[Order By Artist]]",
        OrderByArtistDesc = "[[Order By Artist Descending]]",
        ReleaseDateFilter = "[[Release Date Filter]]",
        VideoDateFilter = "[[Video Date Filter]]",
        ToggleMusicVideo = "[[Music Video Toggle]]",
        ToggleBandLive = "[[Band Live Toggle]]",
        TogglePerformance = "[[Performance Toggle]]",
        ToggleChoreography = "[[Choreography Toggle]]",
        ToggleRelay = "[[Relay Toggle]]",
        ToggleBeOriginal = "[[Be Original Toggle]]",
        ToggleFancam = "[[Fancam Toggle]]",
        ToggleConcert = "[[Concert Toggle]]",
        ToggleMusicShow = "[[Music Show Toggle]]",
        ToggleLiveAudio = "[[Live Audio Toggle]]",
        ToggleRandomPlay = "[[Random Play Toggle]]",
        MaxInPlaylist = "[[Max In Playlist]]",
        MultiAdd = "[[MultiAdd]]",
        ToggleOneVideoPerTrack = "[[One Video/Track]]",
        InvertSelection = "[[Invert Selection]]",
        PrintTopArtistCount = "[[Print Top Artist Count]]",
        SelectFromTxtFile = "[[SelectFromTxtFile]]",
        Clear = "[[Clear]]";

    public static List<string> Actions { get; } =
        [
            Back,
            Quit,
            OrderByName,
            OrderByNameDesc,
            OrderByReleaseDate,
            OrderByReleaseDateDesc,
            OrderByMTime,
            OrderByMTimeDesc,
            OrderByArtist,
            OrderByArtistDesc,
            ReleaseDateFilter,
            VideoDateFilter,
            ToggleMusicVideo,
            ToggleBandLive,
            TogglePerformance,
            ToggleChoreography,
            ToggleRelay,
            ToggleBeOriginal,
            ToggleFancam,
            ToggleConcert,
            ToggleMusicShow,
            ToggleLiveAudio,
            ToggleRandomPlay,
            MaxInPlaylist,
            MultiAdd,
            ToggleOneVideoPerTrack,
            InvertSelection,
            PrintTopArtistCount,
            SelectFromTxtFile,
            Clear,
        ];

    public static List<string> AddActionsToList(List<string> items) =>
        items.Concat(Actions).ToList();

    public static List<string> RemoveActionsFromList(List<string> items) =>
        items.Where(i => !Actions.Contains(i)).ToList();
}
