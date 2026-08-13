using System.Collections.ObjectModel;
using PlaylistMaker.Application;
using PlaylistMaker.Core;
using PlaylistMaker.Infrastructure;
using Terminal.Gui.App;
using Terminal.Gui.Input;
using Terminal.Gui.ViewBase;
using Terminal.Gui.Views;

namespace PlaylistMaker.Tui;

public sealed class MainWindow : Runnable
{
    private readonly TuiServices _services;
    private readonly TuiState _state;
    private readonly MenuBar _menuBar;
    private readonly Label _searchLabel;
    private readonly TextField _search;
    private readonly FrameView _filtersFrame;
    private readonly FrameView _libraryFrame;
    private readonly FrameView _detailsFrame;
    private readonly FrameView _queueFrame;
    private readonly ListView _categoryList;
    private readonly ListView _libraryList;
    private readonly ListView _queueList;
    private readonly Label _details;
    private readonly Label _statusText;
    private readonly ObservableCollection<string> _categoryRows = [];
    private readonly ObservableCollection<string> _libraryRows = [];
    private readonly ObservableCollection<string> _queueRows = [];
    private readonly HistoryFileWatcher _historyWatcher;

    public MainWindow(TuiServices services)
    {
        _services = services;
        _state = services.State;
        Title = "PlaylistMaker TUI";

        _menuBar = BuildMenu();
        _menuBar.TabStop = TabBehavior.NoStop;
        var statusBar = BuildStatusBar();
        var content = new View
        {
            Y = Pos.Bottom(_menuBar),
            Width = Dim.Fill(),
            Height = Dim.Fill(statusBar),
            CanFocus = true,
            TabStop = TabBehavior.TabGroup,
        };

        _searchLabel = new Label { Text = "Search:", X = 1, Y = 0 };
        _search = new TextField
        {
            X = 9,
            Y = 0,
            Width = Dim.Fill(1),
            Text = string.Empty,
        };
        _search.TextChanged += (_, _) =>
        {
            _state.SetSearch(_search.Text.ToString() ?? string.Empty);
            RefreshLibrary();
        };

        _filtersFrame = new FrameView
        {
            Title = "Filters",
            X = 0,
            Y = 1,
            Width = 24,
            Height = Dim.Fill(),
            Visible = false,
        };
        _categoryList = new SearchAwareListView
        {
            Width = Dim.Fill(),
            Height = Dim.Fill(),
            CanFocus = true,
            TabStop = TabBehavior.TabStop,
        };
        _categoryList.SetSource(_categoryRows);
        _categoryList.Accepted += (_, _) => ToggleSelectedCategory();
        ((SearchAwareListView)_categoryList).BeforeKeyDown = key =>
        {
            if (HandleListNavigation(_categoryList, key))
            {
                return true;
            }
            if (key == Key.Space)
            {
                ToggleSelectedCategory();
                return true;
            }
            return RedirectPrintableToSearch(key);
        };
        _filtersFrame.Add(_categoryList);

        _libraryFrame = new FrameView
        {
            Title = "Tracks",
            X = 0,
            Y = 1,
            Width = Dim.Fill(),
            Height = Dim.Fill(),
        };
        _libraryList = new SearchAwareListView
        {
            Width = Dim.Fill(),
            Height = Dim.Fill(),
            CanFocus = true,
            TabStop = TabBehavior.TabStop,
        };
        _libraryList.SetSource(_libraryRows);
        _libraryList.Accepted += (_, _) => ToggleExpansion();
        _libraryList.ValueChanged += (_, _) => RefreshDetails();
        ((SearchAwareListView)_libraryList).BeforeKeyDown = key =>
        {
            if (HandleListNavigation(_libraryList, key))
            {
                return true;
            }
            if (key == Key.L)
            {
                SetSelectedExpansion(true);
                return true;
            }
            if (key == Key.H)
            {
                SetSelectedExpansion(false);
                return true;
            }
            if (key == Key.Space)
            {
                ToggleSelectedQueueItem();
                return true;
            }
            if (key == Key.Enter.WithCtrl)
            {
                PlayQueue();
                return true;
            }
            return RedirectPrintableToSearch(key);
        };
        _libraryFrame.Add(_libraryList);

        _detailsFrame = new FrameView
        {
            Title = "Details / history",
            X = 0,
            Y = 1,
            Width = Dim.Fill(),
            Height = Dim.Percent(55),
            Visible = false,
        };
        _details = new Label
        {
            Width = Dim.Fill(),
            Height = Dim.Fill(),
        };
        _detailsFrame.Add(_details);

        _queueFrame = new FrameView
        {
            Title = "Queue",
            X = 0,
            Y = Pos.Bottom(_detailsFrame),
            Width = Dim.Fill(),
            Height = Dim.Fill(),
            Visible = false,
        };
        _queueList = new SearchAwareListView
        {
            Width = Dim.Fill(),
            Height = Dim.Fill(),
            CanFocus = true,
            TabStop = TabBehavior.TabStop,
        };
        _queueList.SetSource(_queueRows);
        ((SearchAwareListView)_queueList).BeforeKeyDown = key =>
        {
            if (HandleListNavigation(_queueList, key))
            {
                return true;
            }
            if (key == Key.Delete)
            {
                RemoveSelectedQueueItem();
                return true;
            }
            if (key == Key.CursorUp.WithAlt)
            {
                MoveSelectedQueueItem(-1);
                return true;
            }
            if (key == Key.CursorDown.WithAlt)
            {
                MoveSelectedQueueItem(1);
                return true;
            }
            if (key == Key.Enter.WithCtrl)
            {
                PlayQueue();
                return true;
            }
            return RedirectPrintableToSearch(key);
        };
        _queueFrame.Add(_queueList);

        _statusText = new Label
        {
            X = 1,
            Y = Pos.AnchorEnd(1),
            Width = Dim.Fill(1),
            Text = string.Empty,
        };
        content.Add(
            _searchLabel,
            _search,
            _filtersFrame,
            _libraryFrame,
            _detailsFrame,
            _queueFrame,
            _statusText
        );
        Add(_menuBar, content, statusBar);

        _search.KeyDown += (_, key) =>
        {
            if (key == Key.Esc)
            {
                _libraryList.SetFocus();
                key.Handled = true;
            }
            else if (key == Key.J.WithCtrl || key == Key.K.WithCtrl)
            {
                _libraryList.SetFocus();
                MoveSelection(_libraryList, key == Key.J.WithCtrl ? 1 : -1);
                key.Handled = true;
            }
        };
        _search.HasFocusChanged += (_, _) => UpdatePaneTitles();
        _categoryList.HasFocusChanged += (_, _) => UpdatePaneTitles();
        _libraryList.HasFocusChanged += (_, _) => UpdatePaneTitles();
        _queueList.HasFocusChanged += (_, _) => UpdatePaneTitles();

        ViewportChanged += (_, _) => ApplyResponsiveLayout();
        KeyDown += (_, key) =>
        {
            if (key == Key.Enter.WithCtrl)
            {
                PlayQueue();
                key.Handled = true;
            }
            else if (key == Key.O.WithCtrl)
            {
                ShowPlaybackOptions();
                key.Handled = true;
            }
            else if (key == Key.R.WithCtrl)
            {
                Reload();
                key.Handled = true;
            }
            else if (key == Key.Q.WithCtrl)
            {
                App?.RequestStop();
                key.Handled = true;
            }
            else if (key == Key.F1)
            {
                ShowHelp();
                key.Handled = true;
            }
        };

        _historyWatcher = new HistoryFileWatcher(_state.HistoryReader.HistoryPath, () =>
        {
            App?.Invoke(() =>
            {
                _state.RefreshHistory();
                RefreshAll("History refreshed.");
            });
        });

        Initialized += (_, _) =>
        {
            ApplyResponsiveLayout();
            App?.AddTimeout(
                TimeSpan.Zero,
                () =>
                {
                    ShowScriptStatusIfNeeded();
                    FocusLibraryAfterCurrentIteration();
                    return false;
                }
            );
        };
        RefreshAll();
    }

    private MenuBar BuildMenu() => new()
    {
        Menus =
        [
            new MenuBarItem("_File",
            [
                new MenuItem("_Import text playlist", "", ImportTextPlaylist),
                new MenuItem("_Reload", "Ctrl+R", Reload),
                new MenuItem("_Quit", "Ctrl+Q", () => App?.RequestStop()),
            ]),
            new MenuBarItem("_Library",
            [
                new MenuItem("Selected _details", "", ShowSelectedDetails),
                new MenuItem("View _queue", "", ShowQueueDialog),
                new MenuItem("Queue visible _tracks", "", QueueVisibleTracks),
                new MenuItem("Queue matching _videos", "", QueueMatchingVideos),
                new MenuItem("_Clear queue", "", ClearQueue),
                new MenuItem("Top _artists", "", ShowTopArtists),
            ]),
            new MenuBarItem("_Filters",
            [
                new MenuItem("_Categories", "", ShowCategoriesDialog),
                new MenuItem("Track release _date", "", () => ShowDateDialog(true)),
                new MenuItem("Video _date", "", () => ShowDateDialog(false)),
                new MenuItem("_Reset filters", "", ResetFilters),
            ]),
            new MenuBarItem("_Sort",
                Enum.GetValues<LibrarySort>()
                    .Select(sort => new MenuItem(SortLabel(sort), "", () => SetSort(sort)))
                    .ToArray()),
            new MenuBarItem("_Playback",
            [
                new MenuItem("_Options", "Ctrl+O", ShowPlaybackOptions),
                new MenuItem("_Play queue", "Ctrl+Enter", PlayQueue),
            ]),
            new MenuBarItem("_Setup",
            [
                new MenuItem("Install/update mpv history _script", "", InstallMpvScript),
            ]),
            new MenuBarItem("_Help",
            [
                new MenuItem("_Keys", "F1", ShowHelp),
            ]),
        ],
    };

    private StatusBar BuildStatusBar()
    {
        var bar = new StatusBar
        {
            CanFocus = false,
            TabStop = TabBehavior.NoStop,
        };
        bar.Add(new Shortcut(Key.F1, "Help", ShowHelp));
        bar.Add(new Shortcut(Key.Space, "Queue", ToggleSelectedQueueItem));
        bar.Add(new Shortcut(Key.Enter.WithCtrl, "Play", PlayQueue));
        bar.Add(new Shortcut(Key.Q.WithCtrl, "Quit", () => App?.RequestStop()));
        return bar;
    }

    private void RefreshAll(string? status = null)
    {
        RefreshCategories();
        RefreshLibrary();
        RefreshQueue();
        RefreshDetails();
        SetStatus(status ?? "Ready.");
    }

    private void RefreshCategories()
    {
        Replace(_categoryRows, Enum.GetValues<VideoCategory>()
            .Select(category => $"{(_state.Categories.Contains(category) ? "[x]" : "[ ]")} {CategoryLabel(category)}"));
        if (_categoryRows.Count > 0 && (_categoryList.SelectedItem ?? -1) < 0)
        {
            _categoryList.SelectedItem = 0;
        }
    }

    private void RefreshLibrary()
    {
        var selected = _libraryList.SelectedItem ?? 0;
        Replace(_libraryRows, _state.Rows.Select(_state.FormatRow));
        if (_libraryRows.Count > 0)
        {
            _libraryList.SelectedItem = Math.Clamp(selected, 0, _libraryRows.Count - 1);
        }
        UpdatePaneTitles();
        RefreshDetails();
    }

    private void RefreshQueue()
    {
        var selected = _queueList.SelectedItem ?? 0;
        Replace(_queueRows, _state.Queue.Items.Select((item, index) =>
            $"{index + 1,3}. {item.Artist} — {item.Title} [{item.Category}]"));
        if (_queueRows.Count > 0)
        {
            _queueList.SelectedItem = Math.Clamp(selected, 0, _queueRows.Count - 1);
        }
        UpdatePaneTitles();
    }

    private void RefreshDetails() =>
        _details.Text = _state.DetailsFor(_state.RowAt(_libraryList.SelectedItem ?? -1));

    private void ToggleSelectedCategory()
    {
        var index = _categoryList.SelectedItem ?? -1;
        var categories = Enum.GetValues<VideoCategory>();
        if (index < 0 || index >= categories.Length)
        {
            return;
        }

        _state.ToggleCategory(categories[index]);
        RefreshAll();
    }

    private void ToggleExpansion()
    {
        _state.ToggleExpansion(_libraryList.SelectedItem ?? -1);
        RefreshLibrary();
    }

    private void SetSelectedExpansion(bool expanded)
    {
        var selected = _libraryList.SelectedItem ?? -1;
        var selectedRow = _state.RowAt(selected);
        if (selectedRow is null || !_state.SetExpanded(selected, expanded))
        {
            return;
        }

        RefreshLibrary();
        var trackIndex = _state.Rows
            .Select((row, index) => (row, index))
            .First(item => item.row.IsTrack
                && PathIdentity.Comparer.Equals(item.row.Result.Group.Id, selectedRow.Result.Group.Id))
            .index;
        _libraryList.SelectedItem = trackIndex;
        RefreshDetails();
    }

    private void ToggleSelectedQueueItem()
    {
        if (_state.ToggleQueue(_libraryList.SelectedItem ?? -1))
        {
            SetStatus("Added to queue.");
        }
        else
        {
            SetStatus("Removed from queue.");
        }
        RefreshLibrary();
        RefreshQueue();
    }

    private void RemoveSelectedQueueItem()
    {
        _state.Queue.RemoveAt(_queueList.SelectedItem ?? -1);
        RefreshLibrary();
        RefreshQueue();
    }

    private void MoveSelectedQueueItem(int offset)
    {
        var index = _queueList.SelectedItem ?? -1;
        if (_state.Queue.Move(index, offset))
        {
            RefreshQueue();
            _queueList.SelectedItem = index + offset;
        }
    }

    private void QueueVisibleTracks()
    {
        _state.QueueVisibleTracks();
        RefreshAll("Queued one default version for every visible track.");
    }

    private void QueueMatchingVideos()
    {
        _state.QueueMatchingVideos();
        RefreshAll("Queued every matching video.");
    }

    private void ClearQueue()
    {
        _state.Queue.Clear();
        RefreshAll("Queue cleared.");
    }

    private void PlayQueue()
    {
        var result = _services.Playback.Launch(_state.Queue, _state.PlaybackOptions);
        if (!result.Launch.Succeeded)
        {
            MessageBox.ErrorQuery(App!, "Playback failed", result.Launch.Error ?? "Unknown playback error", "OK");
            SetStatus("Playback failed; queue preserved.");
            return;
        }

        RefreshAll($"Launched {result.PlannedVideoCount} video(s) in mpv.");
    }

    private void ImportTextPlaylist()
    {
        var result = new PlaylistTextImporter(_state.Catalog)
            .ImportFile(_services.Config.PlaylistTxtFilePath);
        _state.Queue.AddRange(result.Videos);
        RefreshAll($"Imported {result.Videos.Count} video(s).");
        if (result.Issues.Count > 0)
        {
            var issues = string.Join(Environment.NewLine, result.Issues.Take(12)
                .Select(issue => $"Line {issue.LineNumber}: {issue.Reason} — {issue.Input}"));
            MessageBox.ErrorQuery(App!, "Playlist import issues", issues, "OK");
        }
    }

    private void Reload()
    {
        try
        {
            _state.ReplaceCatalog(_services.LoadCatalog());
            _state.RefreshHistory();
            RefreshAll("Library mappings and history reloaded.");
        }
        catch (Exception exception)
        {
            MessageBox.ErrorQuery(App!, "Reload failed", exception.Message, "OK");
            SetStatus("Reload failed; the current library remains active.");
        }
    }

    private void ResetFilters()
    {
        _state.ResetFilters();
        RefreshAll("Filters reset to official MVs only.");
    }

    private void SetSort(LibrarySort sort)
    {
        _state.SetSort(sort);
        RefreshAll($"Sorted by {SortLabel(sort)}.");
    }

    private void ShowTopArtists()
    {
        var text = string.Join(Environment.NewLine,
            _state.Catalog.TopArtists(30).Select((item, index) => $"{index + 1,2}. {item.Artist}: {item.Count}"));
        MessageBox.Query(App!, "Top artists by video count", text, "OK");
    }

    private void ShowSelectedDetails() => MessageBox.Query(
        App!,
        "Selected track / video",
        _state.DetailsFor(_state.RowAt(_libraryList.SelectedItem ?? -1)),
        "OK"
    );

    private void ShowCategoriesDialog()
    {
        using var dialog = new Dialog { Title = "Video categories", Width = 44, Height = 18 };
        var rows = new ObservableCollection<string>();
        var list = new ListView { X = 1, Y = 1, Width = Dim.Fill(1), Height = Dim.Fill(3) };
        list.SetSource(rows);
        void Refresh()
        {
            Replace(rows, Enum.GetValues<VideoCategory>()
                .Select(category => $"{(_state.Categories.Contains(category) ? "[x]" : "[ ]")} {CategoryLabel(category)}"));
            if (rows.Count > 0 && (list.SelectedItem ?? -1) < 0) list.SelectedItem = 0;
        }
        void Toggle()
        {
            var index = list.SelectedItem ?? -1;
            var categories = Enum.GetValues<VideoCategory>();
            if (index >= 0 && index < categories.Length)
            {
                _state.ToggleCategory(categories[index]);
                Refresh();
            }
        }
        list.Accepted += (_, _) => Toggle();
        list.KeyDown += (_, key) =>
        {
            if (key == Key.Space)
            {
                Toggle();
                key.Handled = true;
            }
        };
        var close = new Button { Text = "_Close", X = Pos.Center(), Y = Pos.AnchorEnd(1) };
        close.Accepted += (_, _) => dialog.RequestStop();
        dialog.Add(list, close);
        Refresh();
        App!.Run(dialog);
        RefreshAll("Category filters updated.");
    }

    private void ShowQueueDialog()
    {
        using var dialog = new Dialog { Title = "Playlist queue", Width = 76, Height = 22 };
        var rows = new ObservableCollection<string>();
        var list = new ListView { X = 1, Y = 1, Width = Dim.Fill(1), Height = Dim.Fill(4) };
        list.SetSource(rows);
        void Refresh()
        {
            Replace(rows, _state.Queue.Items.Select((item, index) =>
                $"{index + 1,3}. {item.Artist} — {item.Title} [{item.Category}]"));
            if (rows.Count > 0) list.SelectedItem = Math.Clamp(list.SelectedItem ?? 0, 0, rows.Count - 1);
        }
        var remove = new Button { Text = "_Remove", X = 2, Y = Pos.AnchorEnd(1) };
        var up = new Button { Text = "_Up", X = Pos.Right(remove) + 1, Y = Pos.AnchorEnd(1) };
        var down = new Button { Text = "_Down", X = Pos.Right(up) + 1, Y = Pos.AnchorEnd(1) };
        var close = new Button { Text = "_Close", X = Pos.AnchorEnd(18), Y = Pos.AnchorEnd(1) };
        var play = new Button { Text = "_Play", X = Pos.AnchorEnd(9), Y = Pos.AnchorEnd(1) };
        remove.Accepted += (_, _) => { _state.Queue.RemoveAt(list.SelectedItem ?? -1); Refresh(); };
        up.Accepted += (_, _) =>
        {
            var index = list.SelectedItem ?? -1;
            if (_state.Queue.Move(index, -1)) { Refresh(); list.SelectedItem = index - 1; }
        };
        down.Accepted += (_, _) =>
        {
            var index = list.SelectedItem ?? -1;
            if (_state.Queue.Move(index, 1)) { Refresh(); list.SelectedItem = index + 1; }
        };
        close.Accepted += (_, _) => dialog.RequestStop();
        play.Accepted += (_, _) => { dialog.RequestStop(); PlayQueue(); };
        dialog.Add(list, remove, up, down, close, play);
        Refresh();
        App!.Run(dialog);
        RefreshAll();
    }

    private void ShowPlaybackOptions()
    {
        using var dialog = new Dialog { Title = "Playback options", Width = 58, Height = 13 };
        var shuffle = new CheckBox
        {
            Text = "_Shuffle",
            X = 2,
            Y = 1,
            Value = _state.PlaybackOptions.Shuffle ? CheckState.Checked : CheckState.UnChecked,
        };
        var onePerTrack = new CheckBox
        {
            Text = "_One video per track",
            X = 2,
            Y = 3,
            Value = _state.PlaybackOptions.OneVideoPerTrack ? CheckState.Checked : CheckState.UnChecked,
        };
        var repeatLabel = new Label { Text = "Repeat each (1-10):", X = 2, Y = 5 };
        var repeat = new TextField { Text = _state.PlaybackOptions.RepeatEach.ToString(), X = 25, Y = 5, Width = 8 };
        var maxLabel = new Label { Text = "Maximum items (0=all):", X = 2, Y = 7 };
        var maximum = new TextField { Text = _state.PlaybackOptions.MaximumItems.ToString(), X = 25, Y = 7, Width = 8 };
        var cancel = new Button { Text = "_Cancel", X = Pos.Center() - 10, Y = 9 };
        var save = new Button { Text = "_Save", X = Pos.Center() + 2, Y = 9 };
        cancel.Accepted += (_, _) => dialog.RequestStop();
        save.Accepted += (_, _) =>
        {
            if (!int.TryParse(repeat.Text.ToString(), out var repeatValue)
                || !int.TryParse(maximum.Text.ToString(), out var maxValue))
            {
                MessageBox.ErrorQuery(App!, "Invalid options", "Repeat and maximum must be whole numbers.", "OK");
                return;
            }

            _state.PlaybackOptions = new PlaybackOptions(
                shuffle.Value == CheckState.Checked,
                maxValue,
                repeatValue,
                onePerTrack.Value == CheckState.Checked
            ).Validate();
            dialog.RequestStop();
        };
        dialog.Add(shuffle, onePerTrack, repeatLabel, repeat, maxLabel, maximum, cancel, save);
        App!.Run(dialog);
        SetStatus("Playback options updated.");
    }

    private void ShowDateDialog(bool trackDate)
    {
        using var dialog = new Dialog { Title = trackDate ? "Track release date" : "Video date", Width = 66, Height = 10 };
        var instructions = new Label { Text = "Date or range: YYYY, YYYY-MM, YYYY-MM-DD, or START..END", X = 2, Y = 1 };
        var input = new TextField { X = 2, Y = 3, Width = Dim.Fill(2) };
        var clear = new Button { Text = "_Clear", X = Pos.Center() - 16, Y = 6 };
        var cancel = new Button { Text = "_Cancel", X = Pos.Center() - 5, Y = 6 };
        var apply = new Button { Text = "_Apply", X = Pos.Center() + 7, Y = 6 };
        clear.Accepted += (_, _) =>
        {
            if (trackDate) _state.SetTrackDate(null); else _state.SetVideoDate(null);
            dialog.RequestStop();
        };
        cancel.Accepted += (_, _) => dialog.RequestStop();
        apply.Accepted += (_, _) =>
        {
            if (!TryParseRange(input.Text.ToString() ?? string.Empty, out var range))
            {
                MessageBox.ErrorQuery(App!, "Invalid date", "Use a supported date or START..END range.", "OK");
                return;
            }
            if (trackDate) _state.SetTrackDate(range); else _state.SetVideoDate(range);
            dialog.RequestStop();
        };
        dialog.Add(instructions, input, clear, cancel, apply);
        App!.Run(dialog);
        RefreshAll("Date filter updated.");
    }

    private void InstallMpvScript()
    {
        try
        {
            _services.ScriptManager.InstallOrUpdate();
            MessageBox.Query(App!, "mpv history script", "The PlaylistMaker mpv script is up to date.", "OK");
        }
        catch (Exception exception)
        {
            MessageBox.ErrorQuery(App!, "Script update failed", exception.Message, "OK");
        }
    }

    private void ShowScriptStatusIfNeeded()
    {
        if (!_services.Config.PlaybackHistoryEnabled)
        {
            return;
        }

        MpvScriptStatus status;
        try
        {
            status = _services.ScriptManager.Status();
        }
        catch (Exception exception)
        {
            MessageBox.ErrorQuery(App!, "Playback history setup", exception.Message, "OK");
            return;
        }
        if (status == MpvScriptStatus.Current)
        {
            return;
        }

        var message = status == MpvScriptStatus.Missing
            ? "The PlaylistMaker mpv history script is not installed. Install it now?"
            : "The installed PlaylistMaker mpv history script is outdated. Update it now?";
        if (MessageBox.Query(App!, "Playback history setup", message, "Later", "Install") == 1)
        {
            InstallMpvScript();
        }
    }

    private void ShowHelp() => MessageBox.Query(
        App!,
        "Keyboard help",
        "Type to search · / focuses search · Esc returns to tracks\n"
        + "Arrows or Ctrl+J/K navigate · H/L collapse/expand · Enter toggles\n"
        + "Space queues · Tab changes pane · Backspace edits search · Ctrl+U clears\n"
        + "Delete removes · Alt+Up/Down reorders\n"
        + "Ctrl+Enter plays · Ctrl+O options · Ctrl+R reloads · Ctrl+Q quits",
        "OK"
    );

    private void ApplyResponsiveLayout()
    {
        var narrow = Viewport.Width < 100;
        _filtersFrame.Visible = !narrow;
        _detailsFrame.Visible = !narrow;
        _queueFrame.Visible = !narrow;
        _libraryFrame.X = narrow ? 0 : Pos.Right(_filtersFrame);
        _libraryFrame.Width = narrow ? Dim.Fill() : Dim.Fill(43);
        _detailsFrame.X = narrow ? 0 : Pos.Right(_libraryFrame);
        _queueFrame.X = narrow ? 0 : Pos.Right(_libraryFrame);
        UpdatePaneTitles();
    }

    private void FocusLibraryAfterCurrentIteration()
    {
        var app = App;
        if (app is null)
        {
            return;
        }

        void FocusLibrary(object? _, Terminal.Gui.App.EventArgs<IApplication?> __)
        {
            app.Iteration -= FocusLibrary;
            _menuBar.InvokeCommand(Command.Quit);
            _libraryList.SetFocus();
            UpdatePaneTitles();
        }

        app.Iteration += FocusLibrary;
    }

    private bool HandleListNavigation(ListView list, Key key)
    {
        if (key == Key.J.WithCtrl)
        {
            MoveSelection(list, 1);
            return true;
        }
        if (key == Key.K.WithCtrl)
        {
            MoveSelection(list, -1);
            return true;
        }
        if (key.AsGrapheme == "/")
        {
            _search.SetFocus();
            return true;
        }
        if (key == Key.Backspace)
        {
            RemoveLastSearchCharacter();
            return true;
        }
        if (key == Key.U.WithCtrl)
        {
            _search.Text = string.Empty;
            return true;
        }

        return false;
    }

    private static void MoveSelection(ListView list, int offset)
    {
        if (offset > 0)
        {
            list.MoveDown(false);
        }
        else
        {
            list.MoveUp(false);
        }
    }

    private bool RedirectPrintableToSearch(Key key)
    {
        var grapheme = key.AsGrapheme;
        if (key.IsAlt || key.IsCtrl || string.IsNullOrEmpty(grapheme)
            || char.IsControl(grapheme[0]))
        {
            return false;
        }

        var nextSearch = (_search.Text.ToString() ?? string.Empty) + grapheme;
        _search.Text = nextSearch;
        key.Handled = true;
        return true;
    }

    private void RemoveLastSearchCharacter()
    {
        var value = _search.Text.ToString() ?? string.Empty;
        if (value.Length > 0)
        {
            _search.Text = value[..^1];
        }
    }

    private void UpdatePaneTitles()
    {
        var narrow = Viewport.Width < 100;
        _searchLabel.Text = _search.HasFocus ? "Search▶" : "Search:";
        _filtersFrame.Title = $"{(_categoryList.HasFocus ? "▶ " : string.Empty)}Filters";
        _libraryFrame.Title = $"{(_libraryList.HasFocus ? "▶ " : string.Empty)}Tracks ({_state.Results.Count})"
            + (narrow ? " — use menus for filters/queue/details" : string.Empty);
        _queueFrame.Title = $"{(_queueList.HasFocus ? "▶ " : string.Empty)}Queue ({_state.Queue.Items.Count})";
    }

    private void SetStatus(string text) =>
        _statusText.Text = $"{text}  Queue: {_state.Queue.Items.Count}  Sort: {SortLabel(_state.Sort)}"
            + "  ·  Ctrl+J/K move · H/L fold · / search";

    private static bool TryParseRange(string input, out DateRange? range)
    {
        range = null;
        var parts = input.Trim().Split("..", StringSplitOptions.TrimEntries);
        if (parts.Length is < 1 or > 2 || string.IsNullOrWhiteSpace(parts[0]))
        {
            return false;
        }

        var start = DateParser.TryParseReleaseDate(parts[0]);
        var end = parts.Length == 2 ? DateParser.TryParseReleaseDate(parts[1]) : start;
        if (start is null || end is null)
        {
            return false;
        }

        range = new DateRange(start, end);
        return true;
    }

    private static void Replace(ObservableCollection<string> target, IEnumerable<string> values)
    {
        target.Clear();
        foreach (var value in values)
        {
            target.Add(value);
        }
    }

    private static string CategoryLabel(VideoCategory category) => category switch
    {
        VideoCategory.MusicVideo => "Music Video",
        VideoCategory.BandLive => "Band Live",
        VideoCategory.BeOriginal => "Be Original",
        VideoCategory.LiveAudio => "Live Audio",
        VideoCategory.MusicShow => "Music Show",
        _ => category.ToString(),
    };

    private static string SortLabel(LibrarySort sort) => sort switch
    {
        LibrarySort.NameAscending => "Title A-Z",
        LibrarySort.NameDescending => "Title Z-A",
        LibrarySort.ArtistAscending => "Artist A-Z",
        LibrarySort.ArtistDescending => "Artist Z-A",
        LibrarySort.TrackReleaseAscending => "Track release oldest",
        LibrarySort.TrackReleaseDescending => "Track release newest",
        LibrarySort.VideoDateAscending => "Video date oldest",
        LibrarySort.VideoDateDescending => "Video date newest",
        LibrarySort.ModifiedAscending => "Modified oldest",
        LibrarySort.ModifiedDescending => "Modified newest",
        _ => sort.ToString(),
    };

    protected override void Dispose(bool disposing)
    {
        if (disposing)
        {
            _historyWatcher.Dispose();
        }
        base.Dispose(disposing);
    }
}

internal sealed class SearchAwareListView : ListView
{
    public Func<Key, bool>? BeforeKeyDown { get; set; }

    protected override bool OnKeyDown(Key key)
    {
        if (BeforeKeyDown?.Invoke(key) == true)
        {
            key.Handled = true;
            return true;
        }

        return base.OnKeyDown(key);
    }
}
