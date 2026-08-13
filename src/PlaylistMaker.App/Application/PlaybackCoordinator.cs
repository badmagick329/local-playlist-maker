using PlaylistMaker.Core;
using PlaylistMaker.Infrastructure;

namespace PlaylistMaker.Application;

public sealed record PlaybackLaunchResult(bool Succeeded, string? Error = null, int? MpvProcessId = null);

public interface IPlaybackCoordinator
{
    PlaybackLaunchResult Launch(PlaybackRequest request);
}

public sealed class PlaybackCoordinator(
    IPlaylistPlayer audioPlayer,
    IPlaylistPlayer videoPlayer,
    PlaybackHistoryService? history = null
) : IPlaybackCoordinator
{
    public PlaybackLaunchResult Launch(PlaybackRequest request)
    {
        if (request.Videos.Count == 0)
        {
            return new PlaybackLaunchResult(false, "The queue is empty.");
        }

        try
        {
            var session = history?.CreateSession(request.Videos, request.Source);
            var audioProcessId = audioPlayer.CreateAndPlay(request.Videos.Select(v => v.Track.FilePath).ToList());
            if (audioProcessId is null)
            {
                return new PlaybackLaunchResult(false, "No playable audio files were found.");
            }

            var mpvProcessId = videoPlayer.CreateAndPlay(
                request.Videos.Select(v => v.FilePath).ToList(),
                session is null ? null : history!.MpvArgumentsFor(session)
            );
            if (mpvProcessId is null)
            {
                return new PlaybackLaunchResult(false, "No playable video files were found.");
            }

            if (session is not null)
            {
                history!.RecordMpvProcess(session, mpvProcessId.Value);
            }

            return new PlaybackLaunchResult(true, MpvProcessId: mpvProcessId);
        }
        catch (Exception exception)
        {
            return new PlaybackLaunchResult(false, exception.Message);
        }
    }
}

public sealed record QueuePlaybackResult(PlaybackLaunchResult Launch, int PlannedVideoCount);

public sealed class QueuePlaybackService(PlaybackPlanner planner, IPlaybackCoordinator coordinator)
{
    public QueuePlaybackResult Launch(
        PlaylistDraft queue,
        PlaybackOptions options,
        string source = "interactive-selection"
    )
    {
        var request = planner.Create(queue.Items, options, source);
        var result = coordinator.Launch(request);
        if (result.Succeeded)
        {
            queue.Clear();
        }

        return new QueuePlaybackResult(result, request.Videos.Count);
    }
}
