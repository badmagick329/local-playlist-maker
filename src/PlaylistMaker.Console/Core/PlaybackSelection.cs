namespace PlaylistMaker.Core;

public record PlaybackSelection(IReadOnlyList<MusicVideo> Videos, string Source);
