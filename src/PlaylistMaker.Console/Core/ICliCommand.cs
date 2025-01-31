namespace PlaylistMaker.Core;

public interface ICliCommand
{
    string Program { get; }
    void SetArgumentSubstitution(string template, string concrete);
    string ParsedArguments();
}