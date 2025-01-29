namespace PlaylistMaker.Tests;

public class GeneralTests
{
    [Theory]
    [MemberData(nameof(SubstringData))]
    public void Substring_BehavesAsExpected(string got, int start, int length, string expected)
    {
        var result = got.Substring(start, length);
        Assert.Equal(expected, result);
    }

    [Fact]
    public void PatternMatching_BehavesAsExpected()
    {
        List<string> choices = ["[[Quit]]"];
        Assert.True(choices is ["[[Back]]"] or ["[[Quit]]"]);
        choices = ["[[Back]]"];
        Assert.True(choices is ["[[Back]]"] or ["[[Quit]]"]);
    }

    public static TheoryData<string, int, int, string> SubstringData()
    {
        return new TheoryData<string, int, int, string>
        {
            { "12345", 0, 5, "12345" },
            { "201108 TWICE - I Can't Stop Me (UHD Live).mp4", 0, 6, "201108" }
        };
    }
}