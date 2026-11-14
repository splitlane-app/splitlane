package rules

import "splitlane/helper/internal/protocol"

// ProgramFolder is the group key of an unpinned program (spec 8.2, 8.4): the program folder with a
// trailing backslash, or the file path itself when the folder is shared (the cases where Normalize
// turns folder into file). The same rule as a pinned folder, so a program keeps its folder when it
// is pinned. A path that Normalize refuses (a pseudo-path such as ":System", not an .exe, not on a
// local drive) is returned as it is. It runs for every connection of every sample, so it does not
// read the file's resources: the name is not needed here.
func ProgramFolder(exe string, env Env) string {
	env.Describe = nil
	n, err := Normalize(protocol.AppRule{Exe: exe, Matcher: protocol.Matcher{Kind: protocol.KindFolder}}, env)
	if err != nil {
		return exe
	}
	return n.Matcher.Value
}
