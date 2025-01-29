using System.Diagnostics;
using PlaylistMaker.Application;

namespace PlaylistMaker.Presentation;

public class FzfReader : IChoicesSelecter
{
    public List<string> AskStringsContainedIn(string prompt, List<string> choices)
    {
        Console.WriteLine(prompt);
        return AskStringsContainedIn(choices);
    }

    public string AskStringContainedIn(string prompt, List<string> choices)
    {
        Console.WriteLine(prompt);
        return AskStringContainedIn(choices);
    }

    public string AskStringContainedIn(List<string> choices)
    {
        using Process process = new Process();
        process.StartInfo.FileName = "fzf.exe";
        process.StartInfo.Arguments =
            "-i --height 60% --reverse --tiebreak=begin";
        process.StartInfo.UseShellExecute = false;
        process.StartInfo.RedirectStandardInput = true;
        process.StartInfo.RedirectStandardOutput = true;
        process.Start();

        using StreamWriter writer = process.StandardInput;
        using StreamReader reader = process.StandardOutput;
        choices.ForEach(writer.WriteLine);

        process.WaitForExit();
        return reader.ReadToEnd().Trim();
    }

    public List<string> AskStringsContainedIn(List<string> choices)
    {
        using Process process = new Process();
        process.StartInfo.FileName = "fzf.exe";
        process.StartInfo.Arguments =
            "-i --multi --height 60% --reverse --tiebreak=begin";
        process.StartInfo.UseShellExecute = false;
        process.StartInfo.RedirectStandardInput = true;
        process.StartInfo.RedirectStandardOutput = true;
        process.Start();

        using StreamWriter writer = process.StandardInput;
        using StreamReader reader = process.StandardOutput;
        choices.ForEach(writer.WriteLine);

        process.WaitForExit();
        var output = reader.ReadToEnd();
        return output
            .Split('\n')
            .Select(l => l.Trim())
            .Where(l => !string.IsNullOrEmpty(l))
            .ToList();
    }
}