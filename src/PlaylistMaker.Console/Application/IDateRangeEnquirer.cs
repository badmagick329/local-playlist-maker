using PlaylistMaker.Core;

namespace PlaylistMaker.Application;

public interface IDateRangeEnquirer
{
    (ReleaseDate? firstDate, ReleaseDate? secondDate)? AskDateRangeOrSingleDate(string prompt);
}