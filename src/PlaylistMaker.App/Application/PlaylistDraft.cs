using PlaylistMaker.Core;

namespace PlaylistMaker.Application;

public sealed class PlaylistDraft
{
    private readonly List<VideoVariant> _items = [];
    private readonly HashSet<string> _ids = new(PathIdentity.Comparer);

    public IReadOnlyList<VideoVariant> Items => _items;

    public bool Contains(VideoVariant variant) => _ids.Contains(variant.Id);

    public bool Toggle(VideoVariant variant)
    {
        if (_ids.Remove(variant.Id))
        {
            var index = _items.FindIndex(item => PathIdentity.Comparer.Equals(item.Id, variant.Id));
            _items.RemoveAt(index);
            return false;
        }

        _items.Add(variant);
        _ids.Add(variant.Id);
        return true;
    }

    public bool Add(VideoVariant variant)
    {
        if (Contains(variant))
        {
            return false;
        }

        _items.Add(variant);
        _ids.Add(variant.Id);
        return true;
    }

    public void AddRange(IEnumerable<VideoVariant> variants)
    {
        foreach (var variant in variants)
        {
            Add(variant);
        }
    }

    public void RemoveAt(int index)
    {
        if (index >= 0 && index < _items.Count)
        {
            _ids.Remove(_items[index].Id);
            _items.RemoveAt(index);
        }
    }

    public bool Move(int index, int offset)
    {
        var target = index + offset;
        if (index < 0 || index >= _items.Count || target < 0 || target >= _items.Count)
        {
            return false;
        }

        var item = _items[index];
        _items.RemoveAt(index);
        _items.Insert(target, item);
        return true;
    }

    public void Clear()
    {
        _items.Clear();
        _ids.Clear();
    }
}
