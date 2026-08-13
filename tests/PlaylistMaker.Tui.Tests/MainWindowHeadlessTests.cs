using PlaylistMaker.Application;
using PlaylistMaker.Core;
using PlaylistMaker.Infrastructure;
using PlaylistMaker.Tui;
using Terminal.Gui.App;
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
    public void LibraryKeyEventsQueueAndRedirectPrintableInputToSearch()
    {
        var (window, state) = CreateWindow();
        using (window)
        {
            var trackFrame = FindFirst<FrameView>(window, frame => frame.Title.StartsWith("Tracks"));
            var list = FindFirst<ListView>(trackFrame, _ => true);

            list.NewKeyDownEvent(Key.Space);
            Assert.Single(state.Queue.Items);

            list.NewKeyDownEvent(Key.W);
            Assert.Equal("w", state.SearchText, ignoreCase: true);

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
    public void RealEventLoopFocusesLibraryAndRoutesKeyboardInput()
    {
        var (window, state) = CreateWindow();
        using var app = Terminal.Gui.App.Application.Create().Init("dotnet");
        app.Driver!.SetScreenSize(120, 40);
        var libraryFocused = false;
        var keyHandled = false;
        var navigationWorked = false;
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
            keyHandled = keyboard.RaiseKeyDownEvent(Key.S);
            app.RequestStop();
        };

        using (window)
        {
            app.Run(window);
        }

        Assert.True(libraryFocused, $"Focused view: {focusedView}");
        Assert.True(navigationWorked);
        Assert.True(keyHandled);
        Assert.Equal("s", state.SearchText, ignoreCase: true);
    }

    private (MainWindow Window, TuiState State) CreateWindow()
    {
        Directory.CreateDirectory(_directory);
        var audio = @"C:\audio\song.flac";
        var map = new Dictionary<string, string>
        {
            [@"C:\videos\240101 Artist - Work.mkv"] = audio,
        };
        var catalog = new MediaLibraryCatalog(new StubReader(audio), map);
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

    private sealed class StubReader(string audioPath) : IVorbisReader
    {
        public VorbisData? VorbisDataFor(string filePath) =>
            new(audioPath, "Artist", "Work", "2024-01-01", 1, "now");
        public List<string> GetAllFilePaths() => [audioPath];
    }

    private sealed class StubCoordinator : IPlaybackCoordinator
    {
        public PlaybackLaunchResult Launch(PlaybackRequest request) => new(true);
    }

    public void Dispose()
    {
        if (Directory.Exists(_directory)) Directory.Delete(_directory, true);
    }
}

[CollectionDefinition("Terminal.Gui event loop", DisableParallelization = true)]
public sealed class TerminalGuiEventLoopCollection;
