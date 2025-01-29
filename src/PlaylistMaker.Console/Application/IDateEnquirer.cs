using PlaylistMaker.Core;

namespace PlaylistMaker.Application;

public interface IDateEnquirer
{
    ReleaseDate? AskDate(string prompt);
}