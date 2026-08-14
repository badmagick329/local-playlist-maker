using Terminal.Gui.Views;

namespace PlaylistMaker.Tui;

internal static class FastListNavigation
{
    public static void Move(ListView list, int offset)
    {
        if (list is SearchAwareListView optimized)
        {
            optimized.MoveSelection(offset);
            return;
        }

        if (offset > 0)
        {
            list.MoveDown(false);
        }
        else
        {
            list.MoveUp(false);
        }
    }
}
