using PlaylistMaker.Application;
using PlaylistMaker.Core;

namespace PlaylistMaker.Tests.Mocks;

public class MockDateRangeEnquirer : IDateRangeEnquirer
{
    public (ReleaseDate? firstDate, ReleaseDate? secondDate)? AskDateRangeOrSingleDate(string prompt)
    {
        return (new ReleaseDate(2016), null);
    }
}