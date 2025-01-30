using System.Diagnostics;
using PlaylistMaker.Application;
using PlaylistMaker.Core;

namespace PlaylistMaker.Infrastructure;

public class UserInputReader : IUserInputReader
{
    public (ReleaseDate? firstDate, ReleaseDate? secondDate)? AskDateRangeOrSingleDate(string prompt)
    {
        Console.WriteLine("Date range: date,date. Single Date: date");
        Console.WriteLine("Valid Date Formats: YYYY, YYYY-MM, YYYY-MM-DD");
        while (true)
        {
            if (!string.IsNullOrEmpty(prompt))
            {
                Console.WriteLine(prompt);
            }

            var dateInput = Console.ReadLine()?.Trim() ?? "";
            if (string.IsNullOrEmpty(dateInput))
            {
                return null;
            }

            if (!dateInput.Contains(','))
            {
                var singleDate = DateParser.TryParseReleaseDate(dateInput);
                if (singleDate is null)
                {
                    Console.WriteLine($"{singleDate} is not a valid date");
                    continue;
                }

                return (DateParser.TryParseReleaseDate(dateInput), null);
            }

            var rangeParts = dateInput.Split(',');
            var (startDateInput, endDateInput) = (rangeParts[0], rangeParts[1]);
            var startDate = DateParser.TryParseReleaseDate(startDateInput.Trim());
            var endDate = DateParser.TryParseReleaseDate(endDateInput.Trim());
            if (startDate is null && endDate is null)
            {
                Console.WriteLine($"{startDateInput} and {endDateInput} are not valid dates");
                continue;
            }

            return (startDate, endDate) switch
            {
                (not null, not null) => (firstDate: startDate, secondDate: endDate),
                (not null, null) => (firstDate: startDate, null),
                (null, not null) => (firstDate: endDate, null),
                _ => throw new Exception("This should not happen")
            };
        }
    }

    public ReleaseDate? AskDate(string prompt)
    {
        ReleaseDate? releaseDate;
        Console.WriteLine("Valid Date Formats: YYYY, YYYY-MM, YYYY-MM-DD");
        while (true)
        {
            if (!string.IsNullOrEmpty(prompt))
            {
                Console.WriteLine(prompt);
            }

            var dateInput = Console.ReadLine()?.Trim() ?? "";
            if (string.IsNullOrEmpty(dateInput))
            {
                return null;
            }

            releaseDate = DateParser.TryParseReleaseDate(dateInput);
            if (releaseDate is not null)
            {
                break;
            }

            Console.WriteLine($"The fuck does {dateInput} mean? Try again or leave blank");
        }

        return releaseDate;
    }

    public string? AskString(string prompt)
    {
        Console.WriteLine(prompt);
        return Console.ReadLine()?.Trim() ?? null;
    }

    public int? AskInt(string prompt)
    {
        Console.WriteLine(prompt);
        while (true)
        {
            var input = Console.ReadLine()?.Trim() ?? null;
            if (input is null)
            {
                return null;
            }

            try
            {
                return int.Parse(input);
            }
            catch
            {
                Console.WriteLine($"Invalid input {input}");
            }
        }
    }


    public List<string> AskStringsContainedIn(string prompt, char separator, List<string> choices)
    {
        var answer = AskString(prompt);
        if (answer is null)
        {
            return [];
        }

        return answer
            .Split(separator)
            .Where(a =>
                choices.Contains(a, StringComparer.InvariantCultureIgnoreCase))
            .ToList();
    }

    public List<string> AskStringsContainedIn(string prompt, List<string> choices)
    {
        Console.WriteLine(prompt);
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