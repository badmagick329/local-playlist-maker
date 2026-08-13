namespace PlaylistMaker.Common;

public static class PathOperations
{
    public static string FixFlacPath(string path)
    {
        // Hacky fix for shitty input
        return path.Replace("/Library/", @"\Library\");
    }
}