package procnet

import "strings"

// bundleSuffix ends the directory name of an application bundle.
const bundleSuffix = ".app"

// exeSuffix ends the file name of a Windows program.
const exeSuffix = ".exe"

// displayName is what a person calls the program at path. A program inside
// an application bundle is named after the outermost bundle, so a helper such
// as Safari.app/Contents/Frameworks/Helper.app/... shows as "Safari", because
// that is the app the person opened. Any other program is named by its file
// name without ".exe". Both separators are accepted so that the rule does not
// depend on the system the path came from.
func displayName(path string) string {
	parts := splitPath(path)
	if len(parts) == 0 {
		return ""
	}
	for _, dir := range parts[:len(parts)-1] {
		if name, ok := strings.CutSuffix(dir, bundleSuffix); ok && name != "" {
			return name
		}
	}
	return trimExe(parts[len(parts)-1])
}

// splitPath cuts path into its non-empty components.
func splitPath(path string) []string {
	return strings.FieldsFunc(path, func(r rune) bool { return r == '/' || r == '\\' })
}

// trimExe drops a case-insensitive ".exe" from the end of file.
func trimExe(file string) string {
	if n := len(file) - len(exeSuffix); n > 0 && strings.EqualFold(file[n:], exeSuffix) {
		return file[:n]
	}
	return file
}
