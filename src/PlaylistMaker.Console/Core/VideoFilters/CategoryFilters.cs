namespace PlaylistMaker.Core.VideoFilters;

public class CategoryFilters
{
    public Dictionary<string, bool> Categories { get; init; }

    public CategoryFilters()
    {
        Categories = CategoryFilterNames.AllCategories().ToDictionary(category => category, _ => false);
    }

    public void MusicVideosOnly()
    {
        foreach (var (category, _) in Categories)
        {
            Categories[category] = category == CategoryFilterNames.MusicVideo;
        }
    }

    public void Update((string category, ToggleType type) actionResult)
    {
        var (category, type) = actionResult;
        switch (type)
        {
            case ToggleType.Show:
                Categories[category] = !Categories[category];
                break;
            case ToggleType.Only:
                foreach (var (key, _) in Categories)
                {
                    if (key == category)
                    {
                        Categories[key] = !Categories[key];
                    }
                }

                break;
            default:
                throw new ArgumentOutOfRangeException($"Invalid toggle type {type}");
        }
    }

    public List<MusicVideo> ToFiltered(List<MusicVideo> musicVideos)
    {
        var filtered = new List<MusicVideo>();
        foreach (var (category, value) in Categories)
        {
            if (!value)
            {
                continue;
            }

            var additionalVideos = category switch
            {
                CategoryFilterNames.BandLive => musicVideos.Where(TypeOfMusicVideo.IsBandLive).ToList(),
                CategoryFilterNames.Performance => musicVideos.Where(TypeOfMusicVideo.IsPerformance).ToList(),
                CategoryFilterNames.Choreography => musicVideos.Where(TypeOfMusicVideo.IsChoreography).ToList(),
                CategoryFilterNames.Relay => musicVideos.Where(TypeOfMusicVideo.IsRelay).ToList(),
                CategoryFilterNames.BeOriginal => musicVideos.Where(TypeOfMusicVideo.IsBeOriginal).ToList(),
                CategoryFilterNames.Fancam => musicVideos.Where(TypeOfMusicVideo.IsFancam).ToList(),
                CategoryFilterNames.Concert => musicVideos.Where(TypeOfMusicVideo.IsConcert).ToList(),
                CategoryFilterNames.MusicShow => musicVideos.Where(TypeOfMusicVideo.IsMusicShow).ToList(),
                CategoryFilterNames.MusicVideo => musicVideos.Where(TypeOfMusicVideo.IsMusicVideo).ToList(),
                CategoryFilterNames.LiveAudio => musicVideos.Where(TypeOfMusicVideo.IsLiveAudio).ToList(),
                _ => []
            };

            foreach (var additionalVideo in additionalVideos.Where(additionalVideo =>
                         !filtered.Contains(additionalVideo)))
            {
                filtered.Add(additionalVideo);
            }
        }

        return filtered;
    }
}