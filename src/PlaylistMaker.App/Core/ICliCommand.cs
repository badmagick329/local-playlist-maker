namespace PlaylistMaker.Core;

public interface ICliCommand
{
    string Program { get; }
    void SetArgumentSubstitution(string template, string concrete);
    IReadOnlyList<string> ParsedArgumentList();
    string ParsedArguments();
    string ArgumentsWith(string arg);
}
