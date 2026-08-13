using System.Collections;
using System.Collections.Specialized;
using System.Globalization;
using PlaylistMaker.Application;
using PlaylistMaker.Core;
using Terminal.Gui.Drawing;
using Terminal.Gui.Text;
using Terminal.Gui.Views;
using TuiAttribute = Terminal.Gui.Drawing.Attribute;

namespace PlaylistMaker.Tui;

internal sealed class LibraryListDataSource(TuiState state) : IListDataSource
{
    private IReadOnlyList<LibraryRow> _rows = [];

    public event NotifyCollectionChangedEventHandler? CollectionChanged;

    public int Count => _rows.Count;

    // Rows are laid out to the viewport width, so horizontal scrolling is neither
    // useful nor representative of the rendered content.
    public int MaxItemLength => 1;

    public bool SuspendCollectionChangedEvent { get; set; }

    public void Replace(IReadOnlyList<LibraryRow> rows)
    {
        _rows = rows;
        Refresh();
    }

    public void Refresh()
    {
        if (!SuspendCollectionChangedEvent)
        {
            CollectionChanged?.Invoke(
                this,
                new NotifyCollectionChangedEventArgs(NotifyCollectionChangedAction.Reset)
            );
        }
    }

    public bool IsMarked(int item) => false;

    public void SetMark(int item, bool value) { }

    public void Dispose() { }

    public bool RenderMark(ListView listView, int item, int row, bool isMarked, bool markMultiple) => false;

    public IList ToList() => _rows.ToList();

    public void Render(
        ListView listView,
        bool selected,
        int item,
        int col,
        int row,
        int width,
        int viewportX
    )
    {
        if (item < 0 || item >= _rows.Count || width <= 0)
        {
            return;
        }

        var baseAttribute = listView.GetAttributeForRole(
            selected ? VisualRole.Focus : VisualRole.Normal
        );
        listView.SetAttribute(baseAttribute);
        listView.Move(col, row);
        listView.AddStr(new string(' ', width));

        var libraryRow = _rows[item];
        if (libraryRow.IsTrack)
        {
            RenderTrack(listView, libraryRow, selected, baseAttribute, col, row, width);
        }
        else
        {
            RenderVariant(listView, libraryRow, selected, baseAttribute, col, row, width);
        }
    }

    private void RenderTrack(
        ListView listView,
        LibraryRow row,
        bool selected,
        TuiAttribute baseAttribute,
        int col,
        int screenRow,
        int width
    )
    {
        var result = row.Result;
        var expanded = state.IsExpanded(result.Group.Id) ? "▼" : "▶";
        var queued = result.EligibleVariants.Any(state.Queue.Contains);
        var queueMark = queued ? "●" : " ";
        var date = result.Group.Track.Date.ToString();
        var count = result.EligibleVariants.Count;
        var metadata = $"{date}  {count}";
        var prefix = $"{expanded} {queueMark} ";
        var gap = "  ";
        var prefixWidth = prefix.GetColumns();
        var metadataWidth = metadata.GetColumns();
        var leftWidth = Math.Max(0, width - prefixWidth - gap.GetColumns() - metadataWidth);

        var artist = result.Group.Track.Artist;
        var title = result.Group.Track.Title;
        const string separator = " — ";
        if ((artist + separator + title).GetColumns() > leftWidth)
        {
            var artistBudget = Math.Min(24, Math.Max(6, leftWidth / 3));
            artist = Clip(artist, artistBudget);
            title = Clip(title, Math.Max(0, leftWidth - artist.GetColumns() - separator.GetColumns()));
        }

        var x = col;
        Draw(listView, ref x, screenRow, prefix[..1], AttributeFor(baseAttribute, selected, ColorName16.DarkGray));
        Draw(listView, ref x, screenRow, " ", baseAttribute);
        Draw(listView, ref x, screenRow, queueMark, AttributeFor(baseAttribute, selected, ColorName16.BrightGreen));
        Draw(listView, ref x, screenRow, " ", baseAttribute);
        Draw(listView, ref x, screenRow, artist, AttributeFor(baseAttribute, selected, ColorName16.BrightCyan));
        Draw(listView, ref x, screenRow, separator, AttributeFor(baseAttribute, selected, ColorName16.DarkGray));
        Draw(listView, ref x, screenRow, title, AttributeFor(baseAttribute, selected, ColorName16.White));

        var metadataX = col + Math.Max(prefixWidth, width - metadataWidth);
        listView.SetAttribute(AttributeFor(baseAttribute, selected, ColorName16.DarkGray));
        listView.AddStr(metadataX, screenRow, metadata);
    }

    private void RenderVariant(
        ListView listView,
        LibraryRow row,
        bool selected,
        TuiAttribute baseAttribute,
        int col,
        int screenRow,
        int width
    )
    {
        var variant = row.Variant!;
        var queued = state.Queue.Contains(variant);
        var prefix = queued ? "   ●  " : "   └  ";
        var details = $"{variant.VideoDate}  {CategoryLabel(variant.Category)}";
        var leftWidth = Math.Max(0, width - details.GetColumns() - prefix.GetColumns() - 2);
        var filename = Clip(variant.FileName, leftWidth);
        var x = col;
        Draw(
            listView,
            ref x,
            screenRow,
            prefix,
            AttributeFor(baseAttribute, selected, queued ? ColorName16.BrightGreen : ColorName16.DarkGray)
        );
        Draw(listView, ref x, screenRow, filename, AttributeFor(baseAttribute, selected, ColorName16.Gray));
        listView.SetAttribute(AttributeFor(baseAttribute, selected, ColorName16.DarkGray));
        listView.AddStr(col + Math.Max(prefix.GetColumns(), width - details.GetColumns()), screenRow, details);
    }

    private static TuiAttribute AttributeFor(
        TuiAttribute baseAttribute,
        bool selected,
        ColorName16 foreground
    ) => selected
        ? baseAttribute
        : new TuiAttribute(new Color(foreground), baseAttribute.Background, baseAttribute.Style);

    private static void Draw(
        ListView listView,
        ref int x,
        int row,
        string text,
        TuiAttribute attribute
    )
    {
        listView.SetAttribute(attribute);
        listView.AddStr(x, row, text);
        x += text.GetColumns();
    }

    private static string Clip(string value, int width)
    {
        if (width <= 0)
        {
            return string.Empty;
        }
        if (value.GetColumns() <= width)
        {
            return value;
        }
        if (width == 1)
        {
            return "…";
        }

        var available = width - 1;
        var used = 0;
        var result = new System.Text.StringBuilder();
        var elements = StringInfo.GetTextElementEnumerator(value);
        while (elements.MoveNext())
        {
            var element = elements.GetTextElement();
            var columns = element.GetColumns();
            if (used + columns > available)
            {
                break;
            }
            result.Append(element);
            used += columns;
        }
        result.Append('…');
        return result.ToString();
    }

    private static string CategoryLabel(VideoCategory category) => category switch
    {
        VideoCategory.MusicVideo => "Music Video",
        VideoCategory.BandLive => "Band Live",
        VideoCategory.LiveAudio => "Live Audio",
        VideoCategory.BeOriginal => "Be Original",
        VideoCategory.MusicShow => "Music Show",
        _ => category.ToString(),
    };
}
