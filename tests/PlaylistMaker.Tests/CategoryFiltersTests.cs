using PlaylistMaker.Core;
using PlaylistMaker.Core.VideoFilters;
using PlaylistMaker.Infrastructure;
using PlaylistMaker.Tests.Mocks;
using PlaylistMaker.View;

namespace PlaylistMaker.Tests;

public class CategoryFiltersTests
{
    [Fact]
    public void Ctor_Initializes_WithDefaults()
    {
        var filterState = new CategoryFilters();
        Assert.NotNull(filterState);
    }

    [Theory]
    [MemberData(nameof(FiltersStateOnlyToggleData))]
    public void FiltersState_Filters_WhenShowOnlyIsToggled(string action, Func<MusicVideo, bool> predicate)
    {
        var musicVideoList = new MockMusicVideoList();
        var videoList = musicVideoList.VideoList();
        var musicVideos = videoList.Select(musicVideoList.MusicVideoFor).ToList();
        var filterState = new CategoryFilters();
        var actionResult = ParsedCategoryFilterAction.ReadAction(action) ?? throw new InvalidOperationException();
        filterState.Update(actionResult);
        var filteredVideos =
            filterState
                .ToFiltered(musicVideos)
                .Select(video => video.FilePath)
                .ToList();

        foreach (var musicVideo in musicVideos)
        {
            if (predicate(musicVideo))
            {
                Assert.Contains(musicVideo.FilePath, filteredVideos);
            }
            else
            {
                Assert.DoesNotContain(musicVideo.FilePath, filteredVideos);
            }
        }

        actionResult = ParsedCategoryFilterAction.ReadAction(action) ?? throw new InvalidOperationException();
        filterState.Update(actionResult);
        filteredVideos =
            filterState
                .ToFiltered(musicVideos)
                .Select(video => video.FilePath)
                .ToList();
        foreach (var musicVideo1 in musicVideos)
        {
            Assert.DoesNotContain(musicVideo1.FilePath, filteredVideos);
        }
    }

    [Theory]
    [MemberData(nameof(FiltersStateOnlyToggleData))]
    public void FiltersState_SelectivelyFilters_WhenShowOnlyIsToggled(string action, Func<MusicVideo, bool> predicate)
    {
        var musicVideoList = new MockMusicVideoList();
        var videoList = musicVideoList.VideoList();
        var musicVideos = videoList.Select(musicVideoList.MusicVideoFor).ToList();
        var filterState = new CategoryFilters();
        // Show all videos 
        ShowAllVideos(filterState);
        var filteredVideos =
            filterState
                .ToFiltered(musicVideos)
                .Select(video => video.FilePath)
                .ToList();

        // All videos are present
        foreach (var musicVideo in musicVideos)
        {
            Assert.Contains(musicVideo.FilePath, filteredVideos);
        }

        // Toggle show with action
        var actionResult = ParsedCategoryFilterAction.ReadAction(action) ?? throw new InvalidOperationException();
        filterState.Update(actionResult);
        filteredVideos =
            filterState
                .ToFiltered(musicVideos)
                .Select(video => video.FilePath)
                .ToList();
        // Only the selected video type is hidden
        foreach (var musicVideo in musicVideos)
        {
            if (predicate(musicVideo))
            {
                Assert.DoesNotContain(musicVideo.FilePath, filteredVideos);
            }
            else
            {
                Assert.Contains(musicVideo.FilePath, filteredVideos);
            }
        }

        // Toggle show with action again
        actionResult = ParsedCategoryFilterAction.ReadAction(action) ?? throw new InvalidOperationException();
        filterState.Update(actionResult);
        filteredVideos =
            filterState
                .ToFiltered(musicVideos)
                .Select(video => video.FilePath)
                .ToList();
        // All videos are present again
        foreach (var musicVideo in musicVideos.Where(predicate))
        {
            Assert.Contains(musicVideo.FilePath, filteredVideos);
        }
    }

    [Theory]
    [MemberData(nameof(FiltersStateToggleData))]
    public void FiltersState_Filters_WhenShowIsToggled(string action, Func<MusicVideo, bool> predicate)
    {
        var musicVideoList = new MockMusicVideoList();
        var videoList = musicVideoList.VideoList();
        var musicVideos = videoList.Select(musicVideoList.MusicVideoFor).ToList();
        var filterState = new CategoryFilters();
        // Show all videos 
        ShowAllVideos(filterState);
        var filteredVideos =
            filterState
                .ToFiltered(musicVideos)
                .Select(video => video.FilePath)
                .ToList();

        // All videos are present
        foreach (var musicVideo in musicVideos)
        {
            Assert.Contains(musicVideo.FilePath, filteredVideos);
        }

        // Toggle show with action
        var actionResult = ParsedCategoryFilterAction.ReadAction(action) ?? throw new InvalidOperationException();
        filterState.Update(actionResult);
        filteredVideos =
            filterState
                .ToFiltered(musicVideos)
                .Select(video => video.FilePath)
                .ToList();
        // Only the selected video type is hidden
        foreach (var musicVideo in musicVideos)
        {
            if (predicate(musicVideo))
            {
                Assert.DoesNotContain(musicVideo.FilePath, filteredVideos);
            }
            else
            {
                Assert.Contains(musicVideo.FilePath, filteredVideos);
            }
        }

        // Toggle show with action again
        actionResult = ParsedCategoryFilterAction.ReadAction(action) ?? throw new InvalidOperationException();
        filterState.Update(actionResult);
        filteredVideos =
            filterState
                .ToFiltered(musicVideos)
                .Select(video => video.FilePath)
                .ToList();
        // All videos are present again
        foreach (var musicVideo in musicVideos.Where(predicate))
        {
            Assert.Contains(musicVideo.FilePath, filteredVideos);
        }
    }

    private void ShowAllVideos(CategoryFilters categoryFilters)
    {
        foreach (var (category, _) in categoryFilters.Categories)
        {
            categoryFilters.Categories[category] = true;
        }
    }

    public static TheoryData<string, Func<MusicVideo, bool>> FiltersStateOnlyToggleData()
    {
        return new TheoryData<string, Func<MusicVideo, bool>>
        {
            { VideoListActions.ToggleMusicVideoOnly, TypeOfMusicVideo.IsMusicVideo },
            { VideoListActions.ToggleBandLiveOnly, TypeOfMusicVideo.IsBandLive },
            { VideoListActions.TogglePerformanceOnly, TypeOfMusicVideo.IsPerformance },
            { VideoListActions.ToggleChoreographyOnly, TypeOfMusicVideo.IsChoreography },
            { VideoListActions.ToggleRelayOnly, TypeOfMusicVideo.IsRelay },
            { VideoListActions.ToggleBeOriginalOnly, TypeOfMusicVideo.IsBeOriginal },
            { VideoListActions.ToggleFancamOnly, TypeOfMusicVideo.IsFancam },
            { VideoListActions.ToggleConcertOnly, TypeOfMusicVideo.IsConcert },
            { VideoListActions.ToggleMusicShowOnly, TypeOfMusicVideo.IsMusicShow },
        };
    }

    public static TheoryData<string, Func<MusicVideo, bool>> FiltersStateToggleData()
    {
        return new TheoryData<string, Func<MusicVideo, bool>>
        {
            { VideoListActions.ToggleMusicVideo, TypeOfMusicVideo.IsMusicVideo },
            { VideoListActions.ToggleBandLive, TypeOfMusicVideo.IsBandLive },
            { VideoListActions.TogglePerformance, TypeOfMusicVideo.IsPerformance },
            { VideoListActions.ToggleChoreography, TypeOfMusicVideo.IsChoreography },
            { VideoListActions.ToggleRelay, TypeOfMusicVideo.IsRelay },
            { VideoListActions.ToggleBeOriginal, TypeOfMusicVideo.IsBeOriginal },
            { VideoListActions.ToggleFancam, TypeOfMusicVideo.IsFancam },
            { VideoListActions.ToggleConcert, TypeOfMusicVideo.IsConcert },
            { VideoListActions.ToggleMusicShow, TypeOfMusicVideo.IsMusicShow },
        };
    }
}