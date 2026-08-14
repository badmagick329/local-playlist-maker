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
    private const string Separator = " — ";
    private static readonly int SeparatorWidth = Separator.GetColumns();
    private static readonly int TrackPrefixWidth = "▶ ● ".GetColumns();
    private static readonly int VariantPrefixWidth = "   ●  ".GetColumns();
    private const int MetadataGapWidth = 2;
    private IReadOnlyList<LibraryRow> _rows = [];
    private RowPresentation?[] _presentations = [];
    private IList _items = Array.Empty<LibraryRow>();
    private readonly Dictionary<(int Item, int Width), RowTextLayout> _layoutCache = [];
    private readonly Dictionary<int, string> _blankRows = [];

    public event NotifyCollectionChangedEventHandler? CollectionChanged;

    public int Count => _rows.Count;

    // Rows are laid out to the viewport width, so horizontal scrolling is neither
    // useful nor representative of the rendered content.
    public int MaxItemLength => 1;

    public bool SuspendCollectionChangedEvent { get; set; }

    public void Replace(IReadOnlyList<LibraryRow> rows)
    {
        _rows = rows;
        _items = rows as IList ?? rows.ToList();
        _presentations = new RowPresentation?[rows.Count];
        _layoutCache.Clear();
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

    public IList ToList() => _items;

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
        listView.AddStr(BlankRow(width));

        var presentation = _presentations[item] ??= CreatePresentation(_rows[item]);
        if (presentation.Row.IsTrack)
        {
            RenderTrack(listView, item, presentation, selected, baseAttribute, col, row, width);
        }
        else
        {
            RenderVariant(listView, item, presentation, selected, baseAttribute, col, row, width);
        }
    }

    private void RenderTrack(
        ListView listView,
        int item,
        RowPresentation presentation,
        bool selected,
        TuiAttribute baseAttribute,
        int col,
        int screenRow,
        int width
    )
    {
        var row = presentation.Row;
        var result = row.Result;
        var expanded = state.IsExpanded(result.Group.Id) ? "▼" : "▶";
        var queued = IsAnyVariantQueued(result.EligibleVariants);
        var queueMark = queued ? "●" : " ";
        var layout = LayoutFor(item, width, presentation);

        var x = col;
        Draw(listView, ref x, screenRow, expanded, 1, AttributeFor(baseAttribute, selected, ColorName16.DarkGray));
        Draw(listView, ref x, screenRow, " ", 1, baseAttribute);
        Draw(listView, ref x, screenRow, queueMark, 1, AttributeFor(baseAttribute, selected, ColorName16.BrightGreen));
        Draw(listView, ref x, screenRow, " ", 1, baseAttribute);
        Draw(listView, ref x, screenRow, layout.Primary, layout.PrimaryWidth, AttributeFor(baseAttribute, selected, ColorName16.BrightCyan));
        Draw(listView, ref x, screenRow, Separator, SeparatorWidth, AttributeFor(baseAttribute, selected, ColorName16.DarkGray));
        Draw(listView, ref x, screenRow, layout.Secondary, layout.SecondaryWidth, AttributeFor(baseAttribute, selected, ColorName16.White));

        var metadataX = col + Math.Max(TrackPrefixWidth, width - presentation.RightWidth);
        listView.SetAttribute(AttributeFor(baseAttribute, selected, ColorName16.DarkGray));
        listView.AddStr(metadataX, screenRow, presentation.RightText);
    }

    private void RenderVariant(
        ListView listView,
        int item,
        RowPresentation presentation,
        bool selected,
        TuiAttribute baseAttribute,
        int col,
        int screenRow,
        int width
    )
    {
        var row = presentation.Row;
        var variant = row.Variant!;
        var queued = state.Queue.Contains(variant);
        var prefix = queued ? "   ●  " : "   └  ";
        var layout = LayoutFor(item, width, presentation);
        var x = col;
        Draw(
            listView,
            ref x,
            screenRow,
            prefix,
            VariantPrefixWidth,
            AttributeFor(baseAttribute, selected, queued ? ColorName16.BrightGreen : ColorName16.DarkGray)
        );
        Draw(listView, ref x, screenRow, layout.Primary, layout.PrimaryWidth, AttributeFor(baseAttribute, selected, ColorName16.Gray));
        listView.SetAttribute(AttributeFor(baseAttribute, selected, ColorName16.DarkGray));
        listView.AddStr(
            col + Math.Max(VariantPrefixWidth, width - presentation.RightWidth),
            screenRow,
            presentation.RightText
        );
    }

    private RowTextLayout LayoutFor(int item, int width, RowPresentation presentation)
    {
        if (_layoutCache.TryGetValue((item, width), out var cached))
        {
            return cached;
        }

        RowTextLayout layout;
        if (presentation.Row.IsTrack)
        {
            var track = presentation.Row.Result.Group.Track;
            var artist = track.Artist;
            var title = track.Title;
            var artistWidth = presentation.PrimaryWidth;
            var titleWidth = presentation.SecondaryWidth;
            var leftWidth = Math.Max(
                0,
                width - TrackPrefixWidth - MetadataGapWidth - presentation.RightWidth
            );
            if (artistWidth + SeparatorWidth + titleWidth > leftWidth)
            {
                var artistBudget = Math.Min(24, Math.Max(6, leftWidth / 3));
                artist = Clip(artist, artistBudget);
                artistWidth = artist.GetColumns();
                title = Clip(title, Math.Max(0, leftWidth - artistWidth - SeparatorWidth));
                titleWidth = title.GetColumns();
            }
            layout = new RowTextLayout(artist, artistWidth, title, titleWidth);
        }
        else
        {
            var fileName = presentation.Row.Variant!.FileName;
            var leftWidth = Math.Max(
                0,
                width - presentation.RightWidth - VariantPrefixWidth - MetadataGapWidth
            );
            if (presentation.PrimaryWidth > leftWidth)
            {
                fileName = Clip(fileName, leftWidth);
            }
            layout = new RowTextLayout(fileName, fileName.GetColumns(), string.Empty, 0);
        }

        _layoutCache[(item, width)] = layout;
        return layout;
    }

    private static RowPresentation CreatePresentation(LibraryRow row)
    {
        if (row.IsTrack)
        {
            var track = row.Result.Group.Track;
            var metadata = $"{track.Date}  {row.Result.EligibleVariants.Count}";
            return new RowPresentation(
                row,
                metadata,
                metadata.GetColumns(),
                track.Artist.GetColumns(),
                track.Title.GetColumns()
            );
        }

        var variant = row.Variant!;
        var details = $"{variant.VideoDate}  {CategoryLabel(variant.Category)}";
        return new RowPresentation(
            row,
            details,
            details.GetColumns(),
            variant.FileName.GetColumns(),
            0
        );
    }

    private bool IsAnyVariantQueued(IReadOnlyList<VideoVariant> variants)
    {
        foreach (var variant in variants)
        {
            if (state.Queue.Contains(variant))
            {
                return true;
            }
        }
        return false;
    }

    private string BlankRow(int width)
    {
        if (!_blankRows.TryGetValue(width, out var blank))
        {
            blank = new string(' ', width);
            _blankRows[width] = blank;
        }
        return blank;
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
        int textWidth,
        TuiAttribute attribute
    )
    {
        listView.SetAttribute(attribute);
        listView.AddStr(x, row, text);
        x += textWidth;
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

    private sealed record RowPresentation(
        LibraryRow Row,
        string RightText,
        int RightWidth,
        int PrimaryWidth,
        int SecondaryWidth
    );

    private sealed record RowTextLayout(
        string Primary,
        int PrimaryWidth,
        string Secondary,
        int SecondaryWidth
    );
}
