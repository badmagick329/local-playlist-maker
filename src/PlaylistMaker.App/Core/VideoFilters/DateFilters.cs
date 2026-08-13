namespace PlaylistMaker.Core.VideoFilters;

public class DateFilters
{
    private ReleaseDate? _trackStartDate;
    private ReleaseDate? _trackEndDate;
    private ReleaseDate? _videoStartDate;
    private ReleaseDate? _videoEndDate;

    public List<MusicVideo> ToFiltered(List<MusicVideo> videos)
    {
        if (_trackStartDate is not null)
        {
            videos = videos
                .Where(v => v.Date
                    .IsAfterDateInclusive(_trackStartDate.Year, _trackStartDate.Month, _trackStartDate.Day)
                )
                .ToList();
        }

        if (_trackEndDate is not null)
        {
            videos = videos
                .Where(v => v.Date
                    .IsBeforeDateInclusive(_trackEndDate.Year, _trackEndDate.Month, _trackEndDate.Day)
                )
                .ToList();
        }

        if (_videoStartDate is not null)
        {
            videos = videos
                .Where(v => v.VideoDate
                    .IsAfterDateInclusive(_videoStartDate.Year, _videoStartDate.Month, _videoStartDate.Day)
                )
                .ToList();
        }

        if (_videoEndDate is not null)
        {
            videos = videos
                .Where(v => v.VideoDate
                    .IsBeforeDateInclusive(_videoEndDate.Year, _videoEndDate.Month, _videoEndDate.Day)
                )
                .ToList();
        }

        return videos;
    }

    public bool TryUpdateTrackReleaseDate((ReleaseDate? firstDate, ReleaseDate? secondDate)? dateInput)
    {
        if (dateInput is null)
        {
            return false;
        }

        var (firstDate, secondDate) = (dateInput.Value.firstDate, dateInput.Value.secondDate);
        switch (firstDate, secondDate)
        {
            case (not null, null):
                _trackStartDate = firstDate;
                _trackEndDate = firstDate;
                break;
            case (null, not null):
                _trackStartDate = secondDate;
                _trackEndDate = secondDate;
                break;
            case (not null, not null):
                _trackStartDate = firstDate;
                _trackEndDate = secondDate;
                break;
            default:
                throw new Exception("This should not happen");
        }

        return true;
    }

    public bool TryUpdateVideoReleaseDate((ReleaseDate? firstDate, ReleaseDate? secondDate)? dateInput)
    {
        if (dateInput is null)
        {
            return false;
        }

        var (firstDate, secondDate) = (dateInput.Value.firstDate, dateInput.Value.secondDate);
        switch (firstDate, secondDate)
        {
            case (not null, null):
                _videoStartDate = firstDate;
                _videoEndDate = firstDate;
                break;
            case (null, not null):
                _videoStartDate = secondDate;
                _videoEndDate = secondDate;
                break;
            case (not null, not null):
                _videoStartDate = firstDate;
                _videoEndDate = secondDate;
                break;
            default:
                throw new Exception("This should not happen");
        }

        return true;
    }

    public void Reset()
    {
        _trackStartDate = null;
        _trackEndDate = null;
        _videoStartDate = null;
        _videoEndDate = null;
    }
}