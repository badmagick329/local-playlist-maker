using PlaylistMaker.Exceptions;

namespace PlaylistMaker.Core;

static class ReleaseDateFactory
{
    public static ReleaseDate? TryFromTagOrPath(string dateStringInTag, string trackPath)
    {
        var dateInPath = ReadDataFromPath(trackPath);
        if (dateInPath is not null)
        {
            return dateInPath;
        }

        var foundReleaseDate = DateParser.TryParseReleaseDate(dateStringInTag);
        return foundReleaseDate;
    }


    public static ReleaseDate FromVideoPath(string videoPath)
    {
        try
        {
            string dateString = Path.GetFileNameWithoutExtension(videoPath)[..6];

            var year = int.Parse($"20{dateString[..2]}");
            var month = int.Parse(dateString[2..4]);
            var day = int.Parse(dateString[4..]);
            return new ReleaseDate(year, month, day);
        }
        catch
        {
            throw new ReleaseDateCreationException($"Could not parse date from {videoPath}");
        }
    }

    private static ReleaseDate? ReadDataFromPath(string trackPath)
    {
        var foundDates = new HashSet<ReleaseDate>();
        DateParser.FindFullDates(trackPath).ForEach(d => foundDates.Add(d));
        return foundDates.Count switch
        {
            > 1 => throw new ReleaseDateCreationException(
                $"More than one valid date found in {trackPath}"),
            1 => foundDates.First(),
            _ => null
        };
    }
}
