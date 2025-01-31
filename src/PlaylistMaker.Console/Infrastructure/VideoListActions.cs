﻿namespace PlaylistMaker.Infrastructure;

public static class VideoListActions
{
    public const string
        Back = "[[Back]]",
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
        ToggleMusicVideoOnly = "[[Music Video Only Toggle]]",
        ToggleBandLive = "[[Band Live Toggle]]",
        ToggleBandLiveOnly = "[[Band Live Only Toggle]]",
        TogglePerformance = "[[Performance Toggle]]",
        TogglePerformanceOnly = "[[Performance Only Toggle]]",
        ToggleChoreography = "[[Choreography Toggle]]",
        ToggleChoreographyOnly = "[[Choreography Only Toggle]]",
        ToggleRelay = "[[Relay Toggle]]",
        ToggleRelayOnly = "[[Relay Only Toggle]]",
        ToggleBeOriginal = "[[Be Original Toggle]]",
        ToggleBeOriginalOnly = "[[Be Original Only Toggle]]",
        ToggleFancam = "[[Fancam Toggle]]",
        ToggleFancamOnly = "[[Fancam Only Toggle]]",
        ToggleConcert = "[[Concert Toggle]]",
        ToggleConcertOnly = "[[Concert Only Toggle]]",
        ToggleMusicShow = "[[Music Show Toggle]]",
        ToggleMusicShowOnly = "[[Music Show Only Toggle]]",
        ToggleLiveAudio = "[[Live Audio Toggle]]",
        ToggleRandomPlay = "[[Random Play Toggle]]",
        MaxInPlaylist = "[[Max In Playlist]]",
        MultiAdd = "[[MultiAdd]]",
        ToggleOneVideoPerTrack = "[[One Video/Track]]",
        InvertSelection = "[[Invert Selection]]",
        PrintTopArtistCount = "[[Print Top Artist Count]]",
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
        // TODO: Deprecated actions, remove at some point?
        // ToggleMusicVideoOnly,
        ToggleBandLive,
        // ToggleBandLiveOnly,
        TogglePerformance,
        // TogglePerformanceOnly,
        ToggleChoreography,
        // ToggleChoreographyOnly,
        ToggleRelay,
        // ToggleRelayOnly,
        ToggleBeOriginal,
        // ToggleBeOriginalOnly,
        ToggleFancam,
        // ToggleFancamOnly,
        ToggleConcert,
        // ToggleConcertOnly,
        ToggleMusicShow,
        // ToggleMusicShowOnly,
        ToggleLiveAudio,
        ToggleRandomPlay,
        MaxInPlaylist,
        MultiAdd,
        ToggleOneVideoPerTrack,
        InvertSelection,
        PrintTopArtistCount,
        Clear,
    ];

    public static List<string> AddActionsToList(List<string> items) => items.Concat(Actions).ToList();

    public static List<string> RemoveActionsFromList(List<string> items) =>
        items.Where(i => !Actions.Contains(i)).ToList();
}