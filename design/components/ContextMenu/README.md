# ContextMenu

Right-click and app menus are **native OS menus**, shown with MyGo's menu API. The design system sets their content and order; the OS draws them, so they look like every other menu on that computer. The drawings are references only.

**The view provides:** the items for the object under the pointer, in the order below.

- **Order.** Copy items first (Copy Port, Copy PID, Copy Address), then navigation (Reveal in the system's process viewer, Filter by This Process), then the destructive item last, after a separator.
- **Text.** One wording on every platform, Title Case. A trailing … means a dialog or picker follows. Shortcuts shown are the real ones.
- **Destructive items** open the Dialog; they never act directly from the menu. Disabled items stay visible and greyed when the reason is obvious; otherwise they are left out.
- Every row of a DataTable, ResultList and sidebar, and every tool card, has one. Text fields get the system's Cut / Copy / Paste menu.
- **App menu.** oh-my-toys (About, Settings…, Quit) · Edit · View (Hide Sidebar, Appearance) · Tools (one item per tool, Mod+1…9) · Window · Help. macOS shows it in the menu bar; elsewhere its items live in the tray panel and the palette.
