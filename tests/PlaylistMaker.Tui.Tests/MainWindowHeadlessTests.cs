using PlaylistMaker.Application;
using PlaylistMaker.Core;
using PlaylistMaker.Infrastructure;
using PlaylistMaker.Tui;
using System.Collections;
using System.Collections.Specialized;
using System.Collections.ObjectModel;
using Terminal.Gui.App;
using Terminal.Gui.Drawing;
using Terminal.Gui.Input;
using Terminal.Gui.ViewBase;
using Terminal.Gui.Views;

namespace PlaylistMaker.Tui.Tests;

[Collection("Terminal.Gui event loop")]
public class MainWindowHeadlessTests : IDisposable
{
    private readonly string _directory = Path.Combine(Path.GetTempPath(), Guid.NewGuid().ToString("N"));

    [Fact]
    public void ConstructsFullWorkspaceAndAcceptExpandsSelectedTrack()
    {
        var (window, state) = CreateWindow();
        using (window)
        {
            Assert.Equal("PlaylistMaker TUI", window.Title);
            var trackFrame = FindFirst<FrameView>(window, frame => frame.Title.StartsWith("Tracks"));
            var list = FindFirst<ListView>(trackFrame, _ => true);

            list.InvokeCommand(Command.Accept);

            Assert.Equal(2, state.Rows.Count);
        }
    }

    [Fact]
    public void NavigationModeQueuesAndIgnoresNonCommandLetters()
    {
        var (window, state) = CreateWindow();
        using (window)
        {
            var trackFrame = FindFirst<FrameView>(window, frame => frame.Title.StartsWith("Tracks"));
            var list = FindFirst<ListView>(trackFrame, _ => true);

            list.NewKeyDownEvent(Key.Space);
            Assert.Single(state.Queue.Items);

            list.NewKeyDownEvent(Key.W);
            Assert.Empty(state.SearchText);

            list.NewKeyDownEvent(Key.Enter.WithCtrl);
            Assert.Empty(state.Queue.Items);
        }
    }

    [Fact]
    public void LibrarySupportsVimStyleMovementAndExplicitExpansion()
    {
        var (window, state) = CreateWindow();
        using (window)
        {
            var trackFrame = FindFirst<FrameView>(window, frame => frame.Title.Contains("Tracks"));
            var list = FindFirst<ListView>(trackFrame, _ => true);

            list.NewKeyDownEvent(Key.L);
            Assert.Equal(2, state.Rows.Count);

            list.NewKeyDownEvent(Key.J.WithCtrl);
            Assert.Equal(1, list.SelectedItem);

            list.NewKeyDownEvent(Key.K.WithCtrl);
            Assert.Equal(0, list.SelectedItem);

            list.NewKeyDownEvent(Key.J.WithCtrl);
            list.NewKeyDownEvent(Key.H);
            Assert.Single(state.Rows);
            Assert.Equal(0, list.SelectedItem);
        }
    }

    [Fact]
    public void LibraryRefreshKeepsItsCustomSourceAndRaisesOneReset()
    {
        var (window, state) = CreateWindow(250);
        using (window)
        {
            var trackFrame = FindFirst<FrameView>(window, frame => frame.Title.Contains("Tracks"));
            var list = FindFirst<ListView>(trackFrame, _ => true);
            var sourceChanges = 0;
            var collectionChanges = 0;
            list.SourceChanged += (_, _) => sourceChanges++;
            list.CollectionChanged += (_, _) => collectionChanges++;

            var search = FindFirst<TextField>(window, _ => true);
            search.Text = "w";

            Assert.Equal(250, state.Rows.Count);
            Assert.Equal(0, sourceChanges);
            Assert.Equal(1, collectionChanges);
        }
    }

    [Fact]
    public void QueueToggleOnlyRefreshesAffectedLibraryRows()
    {
        var (window, state) = CreateWindow(250);
        using (window)
        {
            var trackFrame = FindFirst<FrameView>(window, frame => frame.Title.Contains("Tracks"));
            var list = FindFirst<ListView>(trackFrame, _ => true);
            var sourceChanges = 0;
            var collectionChanges = 0;
            list.SourceChanged += (_, _) => sourceChanges++;
            list.CollectionChanged += (_, _) => collectionChanges++;

            list.NewKeyDownEvent(Key.Space);

            Assert.Single(state.Queue.Items);
            Assert.Equal(0, sourceChanges);
            Assert.Equal(0, collectionChanges);
        }
    }

    [Fact]
    public void CategoryShortcutFocusesPaneAndReturnsToLibrary()
    {
        var (window, _) = CreateWindow();
        using var app = Terminal.Gui.App.Application.Create().Init("dotnet");
        app.Driver!.SetScreenSize(120, 40);
        var worked = false;
        var iterations = 0;
        app.Iteration += (_, _) =>
        {
            iterations++;
            var tracks = FindFirst<FrameView>(window, frame => frame.Title.Contains("Tracks"));
            var filters = FindFirst<FrameView>(window, frame => frame.Title.Contains("Filters"));
            var trackList = FindFirst<ListView>(tracks, _ => true);
            var categoryList = FindFirst<ListView>(filters, _ => true);
            if (!trackList.HasFocus && iterations < 3) return;

            var keyboard = app.Keyboard!;
            keyboard.RaiseKeyDownEvent(Key.C);
            var categoriesFocused = categoryList.HasFocus
                && filters.SchemeName == nameof(Schemes.Accent);
            keyboard.RaiseKeyDownEvent(Key.C);
            worked = categoriesFocused && trackList.HasFocus
                && tracks.SchemeName == nameof(Schemes.Accent);
            app.RequestStop();
        };

        using (window)
        {
            app.Run(window);
        }

        Assert.True(worked);
    }

    [Fact]
    public void EscapeNeverQuitsMainWindowAndGlobalCWorksFromQueue()
    {
        var (window, _) = CreateWindow();
        using var app = Terminal.Gui.App.Application.Create().Init("dotnet");
        app.Driver!.SetScreenSize(120, 40);
        var worked = false;
        var iterations = 0;
        app.Iteration += (_, _) =>
        {
            iterations++;
            var tracks = FindFirst<FrameView>(window, frame => frame.Title.Contains("Tracks"));
            var filters = FindFirst<FrameView>(window, frame => frame.Title.Contains("Filters"));
            var queue = FindFirst<FrameView>(window, frame => frame.Title.Contains("Queue"));
            var trackList = FindFirst<ListView>(tracks, _ => true);
            var categoryList = FindFirst<ListView>(filters, _ => true);
            var queueList = FindFirst<ListView>(queue, _ => true);
            if (!trackList.HasFocus && iterations < 3) return;

            queueList.SetFocus();
            app.Keyboard!.RaiseKeyDownEvent(Key.C);
            var globalCategoryFocus = categoryList.HasFocus;
            app.Keyboard.RaiseKeyDownEvent(Key.Esc);
            var escapeReturnedToTracks = trackList.HasFocus && window.IsRunning;
            app.Keyboard.RaiseKeyDownEvent(Key.Esc);
            worked = globalCategoryFocus && escapeReturnedToTracks && window.IsRunning;
            app.RequestStop();
        };

        using (window)
        {
            app.Run(window);
        }

        Assert.True(worked);
    }

    [Fact]
    public void PlaybackOptionsSupportsVimStyleFocusNavigation()
    {
        var (window, _) = CreateWindow();
        using var app = Terminal.Gui.App.Application.Create().Init("dotnet");
        app.Driver!.SetScreenSize(120, 40);
        var worked = false;
        var opened = false;
        app.Iteration += (_, _) =>
        {
            if (opened)
            {
                return;
            }
            opened = true;
            app.AddTimeout(TimeSpan.Zero, () =>
            {
                var dialog = Assert.IsType<Dialog>(app.TopRunnableView);
                var textFields = Descendants(dialog).OfType<TextField>().ToList();
                var repeat = textFields[0];
                var maximum = textFields[1];
                var originalRepeat = repeat.Text.ToString();

                app.Keyboard!.RaiseKeyDownEvent(Key.J);
                app.Keyboard.RaiseKeyDownEvent(Key.J);
                var plainJReachedRepeat = repeat.HasFocus && repeat.Text.ToString() == originalRepeat;
                app.Keyboard.RaiseKeyDownEvent(Key.J);
                var plainJLeftTextField = maximum.HasFocus && repeat.Text.ToString() == originalRepeat;
                app.Keyboard.RaiseKeyDownEvent(Key.K.WithCtrl);
                var controlKReturned = repeat.HasFocus;
                app.Keyboard.RaiseKeyDownEvent(Key.J.WithCtrl);
                var controlJAdvanced = maximum.HasFocus;

                worked = plainJReachedRepeat && plainJLeftTextField && controlKReturned && controlJAdvanced;
                app.RequestStop(dialog);
                return false;
            });

            app.Keyboard!.RaiseKeyDownEvent(Key.O.WithCtrl);
            app.RequestStop();
        };

        using (window)
        {
            app.Run(window);
        }

        Assert.True(worked);
    }

    [Fact]
    public void EnterSavesRepeatOptionAndShowsPlannedQueueCount()
    {
        var (window, state) = CreateWindow();
        state.Queue.Add(state.Results[0].DefaultVariant);
        using var app = Terminal.Gui.App.Application.Create().Init("dotnet");
        app.Driver!.SetScreenSize(120, 40);
        var opened = false;
        app.Iteration += (_, _) =>
        {
            if (opened)
            {
                return;
            }
            opened = true;
            app.AddTimeout(TimeSpan.Zero, () =>
            {
                var dialog = Assert.IsType<Dialog>(app.TopRunnableView);
                var repeat = Descendants(dialog).OfType<TextField>().First();
                repeat.Text = "3";
                repeat.SetFocus();
                app.Keyboard!.RaiseKeyDownEvent(Key.Enter);
                return false;
            });

            app.Keyboard!.RaiseKeyDownEvent(Key.O.WithCtrl);
            app.RequestStop();
        };

        string queueTitle;
        using (window)
        {
            app.Run(window);
            queueTitle = FindFirst<FrameView>(window, frame => frame.Title.Contains("Queue")).Title;
        }

        Assert.Equal(3, state.PlaybackOptions.RepeatEach);
        Assert.Contains("1 → 3 plays", queueTitle);
    }

    [Fact]
    public void RepeatedNavigationRendersOnlyRowsWhoseSelectionChanged()
    {
        using var app = Terminal.Gui.App.Application.Create().Init("dotnet");
        app.Driver!.SetScreenSize(80, 25);
        using var window = new Window { Width = Dim.Fill(), Height = Dim.Fill() };
        var list = new SearchAwareListView
        {
            Width = 40,
            Height = 10,
            OptimizeSelectionDrawing = true,
        };
        list.SetSource(new ObservableCollection<string>(Enumerable.Range(0, 50).Select(i => $"Track {i}")));
        var counter = new CountingListDataSource(list.Source!);
        list.Source = counter;
        list.SelectedItem = 2;
        window.Add(list);

        var phase = 0;
        var renderedRows = -1;
        app.Iteration += (_, _) =>
        {
            if (phase == 0)
            {
                counter.RenderCount = 0;
                FastListNavigation.Move(list, 1);
                FastListNavigation.Move(list, 1);
                phase = 1;
                return;
            }

            renderedRows = counter.RenderCount;
            app.RequestStop();
        };

        app.Run(window);
        Assert.Equal(4, list.SelectedItem);
        Assert.InRange(renderedRows, 1, 3);
    }

    [Fact]
    public void InitialLayoutIsSafeBeforeTerminalSizeIsKnown()
    {
        var (window, _) = CreateWindow();
        using (window)
        {
            var exception = Record.Exception(() => window.Layout(new System.Drawing.Size(40, 20)));

            Assert.Null(exception);
            Assert.All(
                Descendants(window),
                view =>
                {
                    Assert.True(view.Frame.Width >= 0);
                    Assert.True(view.Frame.Height >= 0);
                }
            );
        }
    }

    [Fact]
    public void RealEventLoopSeparatesNavigationAndSearchModes()
    {
        var (window, state) = CreateWindow();
        using var app = Terminal.Gui.App.Application.Create().Init("dotnet");
        app.Driver!.SetScreenSize(120, 40);
        var libraryFocused = false;
        var keyHandled = false;
        var navigationWorked = false;
        var commandsDidNotSearch = false;
        var searchModeWorked = false;
        var modeWasVisible = false;
        string? focusedView = null;
        var iterationCount = 0;
        app.Iteration += (_, _) =>
        {
            iterationCount++;
            var trackFrame = FindFirst<FrameView>(window, frame => frame.Title.Contains("Tracks"));
            var list = FindFirst<ListView>(trackFrame, _ => true);
            libraryFocused = list.HasFocus;
            var focused = app.Navigation?.GetFocused();
            focusedView = focused is null
                ? $"none; chain={DescribeFocusChain(list)}"
                : $"{focused.GetType().Name} title='{focused!.Title}' text='{focused.Text}'";
            if (!libraryFocused && iterationCount < 3)
            {
                return;
            }

            var keyboard = app.Keyboard!;
            keyboard.RaiseKeyDownEvent(Key.L);
            var expanded = state.Rows.Count == 2;
            keyboard.RaiseKeyDownEvent(Key.CursorDown);
            var arrowDown = list.SelectedItem == 1;
            keyboard.RaiseKeyDownEvent(Key.CursorUp);
            var arrowUp = list.SelectedItem == 0;
            keyboard.RaiseKeyDownEvent(Key.J.WithCtrl);
            var controlJ = list.SelectedItem == 1;
            keyboard.RaiseKeyDownEvent(Key.K.WithCtrl);
            var controlK = list.SelectedItem == 0;
            navigationWorked = expanded && arrowDown && arrowUp && controlJ && controlK;
            keyboard.RaiseKeyDownEvent(Key.H);
            keyboard.RaiseKeyDownEvent(Key.L);
            commandsDidNotSearch = string.IsNullOrEmpty(state.SearchText);
            var status = Descendants(window).OfType<Label>()
                .First(label => label.Text.ToString()?.Contains("[NAV]") == true);
            var navigationWasVisible = trackFrame.SchemeName == nameof(Schemes.Accent)
                && status.Text.ToString()?.Contains("[NAV]") == true;

            keyHandled = keyboard.RaiseKeyDownEvent(new Key('/'));
            var search = FindFirst<TextField>(window, _ => true);
            keyboard.RaiseKeyDownEvent(Key.H);
            keyboard.RaiseKeyDownEvent(Key.Space);
            keyboard.RaiseKeyDownEvent(Key.L);
            var textWasEntered = search.HasFocus
                && search.Text.ToString()?.Equals("h l", StringComparison.OrdinalIgnoreCase) == true;
            var searchWasVisible = search.SchemeName == nameof(Schemes.Accent)
                && status.Text.ToString()?.Contains("[SEARCH]") == true;
            modeWasVisible = navigationWasVisible && searchWasVisible;
            keyboard.RaiseKeyDownEvent(Key.Esc);
            searchModeWorked = textWasEntered
                && state.SearchText.Equals("h l", StringComparison.OrdinalIgnoreCase)
                && list.HasFocus;
            app.RequestStop();
        };

        using (window)
        {
            app.Run(window);
        }

        Assert.True(libraryFocused, $"Focused view: {focusedView}");
        Assert.True(navigationWorked);
        Assert.True(commandsDidNotSearch);
        Assert.True(keyHandled);
        Assert.True(searchModeWorked);
        Assert.True(modeWasVisible);
        Assert.Equal("h l", state.SearchText, ignoreCase: true);
        Assert.Empty(state.Queue.Items);
    }

    private (MainWindow Window, TuiState State) CreateWindow(int trackCount = 1)
    {
        Directory.CreateDirectory(_directory);
        var audioPaths = Enumerable.Range(0, trackCount)
            .Select(index => $@"C:\audio\song-{index:0000}.flac")
            .ToList();
        var map = audioPaths
            .Select((audio, index) => (audio, video: $@"C:\videos\240101 Artist - Work {index:0000}.mkv"))
            .ToDictionary(item => item.video, item => item.audio);
        var catalog = new MediaLibraryCatalog(new StubReader(audioPaths), map);
        var state = new TuiState(catalog, new PlaybackHistoryReader(Path.Combine(_directory, "history.jsonl")));
        var config = new Config
        {
            DataDirectory = _directory,
            PlaylistTxtFilePath = Path.Combine(_directory, "playlist.txt"),
        };
        var services = new TuiServices(
            config,
            state,
            new QueuePlaybackService(new PlaybackPlanner(), new StubCoordinator()),
            () => catalog,
            new MpvScriptManager(Path.Combine(_directory, "mpv", "playlistmaker-history.lua"))
        );
        return (new MainWindow(services), state);
    }

    private static T FindFirst<T>(View root, Func<T, bool> predicate) where T : View
    {
        foreach (var view in Descendants(root))
        {
            if (view is T typed && predicate(typed)) return typed;
        }
        throw new Xunit.Sdk.XunitException($"No {typeof(T).Name} matched the predicate.");
    }

    private static IEnumerable<View> Descendants(View view)
    {
        foreach (var subView in view.SubViews)
        {
            yield return subView;
            foreach (var descendant in Descendants(subView)) yield return descendant;
        }
    }

    private static string DescribeFocusChain(View view)
    {
        var parts = new List<string>();
        for (View? current = view; current is not null; current = current.SuperView)
        {
            parts.Add($"{current.GetType().Name}(CanFocus={current.CanFocus},Visible={current.Visible},Enabled={current.Enabled},TabStop={current.TabStop})");
        }
        return string.Join(" <- ", parts);
    }

    private sealed class StubReader(IReadOnlyList<string> audioPaths) : IVorbisReader
    {
        public VorbisData? VorbisDataFor(string filePath) =>
            new(filePath, "Artist", $"Work {Path.GetFileNameWithoutExtension(filePath)}", "2024-01-01", 1, "now");
        public List<string> GetAllFilePaths() => audioPaths.ToList();
    }

    private sealed class StubCoordinator : IPlaybackCoordinator
    {
        public PlaybackLaunchResult Launch(PlaybackRequest request) => new(true);
    }

    private sealed class CountingListDataSource(IListDataSource inner) : IListDataSource
    {
        public int RenderCount { get; set; }
        public int Count => inner.Count;
        public int MaxItemLength => inner.MaxItemLength;
        public bool SuspendCollectionChangedEvent
        {
            get => inner.SuspendCollectionChangedEvent;
            set => inner.SuspendCollectionChangedEvent = value;
        }
        public event NotifyCollectionChangedEventHandler? CollectionChanged
        {
            add => inner.CollectionChanged += value;
            remove => inner.CollectionChanged -= value;
        }

        public bool IsMarked(int item) => inner.IsMarked(item);
        public void SetMark(int item, bool value) => inner.SetMark(item, value);
        public bool RenderMark(ListView listView, int item, int row, bool isMarked, bool markMultiple) =>
            inner.RenderMark(listView, item, row, isMarked, markMultiple);
        public IList ToList() => inner.ToList();
        public void Render(ListView listView, bool selected, int item, int col, int row, int width, int viewportX)
        {
            RenderCount++;
            inner.Render(listView, selected, item, col, row, width, viewportX);
        }
        public void Dispose() { }
    }

    public void Dispose()
    {
        if (Directory.Exists(_directory)) Directory.Delete(_directory, true);
    }
}

[CollectionDefinition("Terminal.Gui event loop", DisableParallelization = true)]
public sealed class TerminalGuiEventLoopCollection;
