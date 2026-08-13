using PlaylistMaker.Core;

namespace PlaylistMaker.Application;

public static class VideoCategoryClassifier
{
    public static VideoCategory Classify(MusicVideo video)
    {
        if (TypeOfMusicVideo.IsBandLive(video)) return VideoCategory.BandLive;
        if (TypeOfMusicVideo.IsPerformance(video)) return VideoCategory.Performance;
        if (TypeOfMusicVideo.IsChoreography(video)) return VideoCategory.Choreography;
        if (TypeOfMusicVideo.IsRelay(video)) return VideoCategory.Relay;
        if (TypeOfMusicVideo.IsBeOriginal(video)) return VideoCategory.BeOriginal;
        if (TypeOfMusicVideo.IsFancam(video)) return VideoCategory.Fancam;
        if (TypeOfMusicVideo.IsConcert(video)) return VideoCategory.Concert;
        if (TypeOfMusicVideo.IsMusicShow(video)) return VideoCategory.MusicShow;
        if (TypeOfMusicVideo.IsRemix(video)) return VideoCategory.Remix;
        if (TypeOfMusicVideo.IsLiveAudio(video)) return VideoCategory.LiveAudio;
        return VideoCategory.MusicVideo;
    }
}
