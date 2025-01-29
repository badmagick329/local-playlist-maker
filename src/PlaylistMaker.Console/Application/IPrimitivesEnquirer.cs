namespace PlaylistMaker.Application;

public interface IPrimitivesEnquirer
{
    string? AskString(string prompt);
    int? AskInt(string prompt);
    List<string> AskStringsContainedIn(string prompt, char separator, List<string> choices);
    List<string> AskStringsContainedIn(string prompt, List<string> choices);
}