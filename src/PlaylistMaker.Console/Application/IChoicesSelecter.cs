namespace PlaylistMaker.Application;

public interface IChoicesSelecter
{
    string AskStringContainedIn(string prompt, List<string> choices);
    string AskStringContainedIn(List<string> choices);
    List<string> AskStringsContainedIn(string prompt, List<string> choices);
    List<string> AskStringsContainedIn(List<string> choices);
}