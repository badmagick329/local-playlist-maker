using PlaylistMaker.Core;

namespace PlaylistMaker.Application;

public sealed record PlaylistImportIssue(int LineNumber, string Input, string Reason);

public sealed record PlaylistImportResult(
    IReadOnlyList<VideoVariant> Videos,
    IReadOnlyList<PlaylistImportIssue> Issues
);

public sealed class PlaylistTextImporter(MediaLibraryCatalog catalog)
{
    public PlaylistImportResult ImportFile(string path)
    {
        if (!File.Exists(path))
        {
            return new PlaylistImportResult([], [new PlaylistImportIssue(0, path, "File not found")]);
        }

        return ImportLines(File.ReadLines(path));
    }

    public PlaylistImportResult ImportLines(IEnumerable<string> lines)
    {
        var videos = new List<VideoVariant>();
        var issues = new List<PlaylistImportIssue>();
        var lineNumber = 0;
        foreach (var raw in lines)
        {
            lineNumber++;
            var input = raw.Trim();
            if (string.IsNullOrWhiteSpace(input) || input.StartsWith('#'))
            {
                continue;
            }

            VideoVariant? resolved = null;
            if (Path.IsPathFullyQualified(input))
            {
                resolved = catalog.FindByPath(input);
            }

            if (resolved is null)
            {
                var byName = catalog.FindByFileName(input);
                if (byName.Count == 1)
                {
                    resolved = byName[0];
                }
                else if (byName.Count > 1)
                {
                    issues.Add(new PlaylistImportIssue(lineNumber, input, "Filename is ambiguous"));
                    continue;
                }
            }

            if (resolved is null)
            {
                issues.Add(new PlaylistImportIssue(lineNumber, input, "No matching video"));
                continue;
            }

            videos.Add(resolved);
        }

        return new PlaylistImportResult(videos, issues);
    }
}
