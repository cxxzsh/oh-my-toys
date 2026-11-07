# ResultList

The preview of an action tool's Plan: every change as original → result, with conflicts, and the Apply button.

**The view provides:** the Plan's items with their outcome (ready, conflict, skipped), the count, the Apply verb.

- Rows as DataTable, with a centred `→` column. Conflicts: row `warning-soft`, new name in `warning`, a Conflict badge; they are skipped, never silently overwritten.
- Sticky footer (`bg-titlebar`, `line` above): the summary with counts, then Cancel and the primary button with the real count ("Rename 3 files"), in the same order as the Dialog.
- Apply runs off the main thread; the list turns into progress, then a result line with Undo where the tool supports it.
- The Plan changes nothing: editing the pattern only recomputes this list.
