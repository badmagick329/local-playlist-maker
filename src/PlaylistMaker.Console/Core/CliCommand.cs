﻿using System.Text;

namespace PlaylistMaker.Core;

public class CliCommand : ICliCommand
{
    public string Program { get; }
    private IEnumerable<string> Arguments { get; }
    private readonly Dictionary<string, string> _templateSubstitutions;

    private CliCommand(string program, IEnumerable<string> arguments)
    {
        Program = program;
        Arguments = arguments;
        _templateSubstitutions = new Dictionary<string, string>();
    }

    public static CliCommand CreateFromList(IEnumerable<string> command)
    {
        var commandArray = command as string[] ?? command.ToArray();
        if (commandArray.Length == 0)
        {
            throw new ArgumentException("Command cannot be null or empty");
        }

        return new CliCommand(commandArray.First(), commandArray.Skip(1));
    }

    public void SetArgumentSubstitution(string template, string concrete) =>
        _templateSubstitutions[template] = concrete;

    public string ParsedArguments() =>
        string.Join(' ',
            Arguments.Select(arg =>
            {
                var argBuilder = new StringBuilder(arg);
                foreach (var substitution in _templateSubstitutions)
                {
                    argBuilder.Replace(substitution.Key, substitution.Value);
                }

                return argBuilder.ToString();
            })
        );
}