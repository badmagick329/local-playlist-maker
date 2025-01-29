using System.Globalization;
using System.Text;
using System.Text.Json;
using FlacLibSharp;
using PlaylistMaker.Core;
using PlaylistMaker.Exceptions;

namespace PlaylistMaker.Infrastructure;

public class VorbisReader : IVorbisReader
{
    private IFlacPathsReader FlacsReader { get; }
    private string CachePath { get; }

    private List<VorbisData> Data { get; set; } = [];

    public VorbisReader(IFlacPathsReader flacsReader, string cachePath)
    {
        FlacsReader = flacsReader;
        CachePath = cachePath;

        if (File.Exists(cachePath))
        {
            return;
        }

        Console.WriteLine($"Cache file does not exist. Creating {cachePath}");
        PopulateData();
    }

    public VorbisData? VorbisDataFor(string filePath)
    {
        EnsureDataIsPresent();

        var foundData = Data.Find(d => d.FilePath == filePath);
        if (foundData is not null)
        {
            return foundData;
        }

        var vorbisComment = ReadVorbisComment(filePath);
        return vorbisComment is null
            ? null
            : CreateVorbisDataFromVorbisCommentAndSave(filePath, vorbisComment);
    }

    public List<string> GetAllFilePaths()
    {
        EnsureDataIsPresent();
        return Data.Select(d => d.FilePath).ToList();
    }

    private void EnsureDataIsPresent()
    {
        if (Data.Count == 0)
        {
            PopulateData();
        }

        if (Data.Count == 0)
        {
            throw new FlacReaderException("No data found");
        }
    }

    private void PopulateData()
    {
        if (Data.Count > 0)
        {
            return;
        }

        if (File.Exists((CachePath)))
        {
            Data = JsonSerializer.Deserialize<List<VorbisData>>(
                File.ReadAllText(CachePath), new JsonSerializerOptions
                {
                    PropertyNameCaseInsensitive = true
                }) ?? throw new FlacReaderException(
                "Failed to deserialize cache file");

            return;
        }


        var flacPaths = FlacsReader.ReadFlacPaths();
        Data = flacPaths
            .Select(filePath => new
                { filePath, comment = ReadVorbisComment(filePath) })
            .Where(x => x.comment is not null)
            .Select(x =>
                CreateVorbisDataFromVorbisComment(x.filePath, x.comment!))
            .ToList();
        Save();
    }


    private static VorbisComment? ReadVorbisComment(string file)
    {
        try
        {
            using FlacFile flacFile = new(file);
            return flacFile.VorbisComment;
        }
        catch
        {
            return null;
        }
    }


    private VorbisData CreateVorbisDataFromVorbisCommentAndSave(string filePath,
        VorbisComment vorbisComment)
    {
        var newVorbisData =
            CreateVorbisDataFromVorbisComment(filePath, vorbisComment);
        Data.Add(newVorbisData);
        Save();
        return newVorbisData;
    }

    private static VorbisData CreateVorbisDataFromVorbisComment(string filePath,
        VorbisComment vorbisComment)
    {
        var releaseDate =
            DateParser.TryParseReleaseDate(vorbisComment.Date.Value);
        var date = releaseDate is not null ? releaseDate.AsString : "";
        var now = DateTime.Now.ToString(CultureInfo.InvariantCulture);
        int trackNumber =
            int.TryParse(vorbisComment.TrackNumber.Value, out var number)
                ? number
                : 1;
        return new VorbisData(filePath, vorbisComment.Artist.Value,
            vorbisComment.Title.Value,
            date, trackNumber, now);
    }

    private void Save()  {
        
        var parent = Directory.GetParent(CachePath);
        if (parent is not null && !parent.Exists) {
            Directory.CreateDirectory(parent.FullName);
        }
        
        File.WriteAllText(CachePath,
        JsonSerializer.Serialize(Data), Encoding.UTF8);}
}