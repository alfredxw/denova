package toolapproval

import "strings"

// These classifiers cover read commands whose option grammar can otherwise
// introduce hidden file reads, writes, or executable helpers. Unknown shapes
// deliberately fall back to approval instead of growing a permissive generic
// flag parser.
func safePathReadArguments(name string, args []string, boundary pathBoundary) bool {
	for index, arg := range args {
		lower := strings.ToLower(arg)
		switch name {
		case "ls", "tree":
			if name == "ls" && hasShortOption(arg, 'L') ||
				name == "tree" && hasShortOption(arg, 'l') ||
				lower == "--dereference" || lower == "--followlinks" {
				return false
			}
		case "file":
			if lower == "-f" || strings.HasPrefix(lower, "--files-from=") || lower == "--files-from" {
				return false
			}
			if strings.HasPrefix(arg, "-f") && arg != "-f" && !strings.HasPrefix(arg, "--") {
				return false
			}
		case "du":
			if hasShortOption(arg, 'L') || hasShortOption(arg, 'H') ||
				lower == "--dereference" || lower == "--dereference-args" ||
				lower == "--files0-from" || strings.HasPrefix(lower, "--files0-from=") ||
				optionPathInside(args, index, lower, "--exclude-from", boundary) == optionPathOutside {
				return false
			}
		}
	}
	paths := nonFlagArguments(args)
	return allPathsInside(boundary, paths) || name == "df" && len(paths) == 0
}

func safeStreamReadArguments(name string, args []string, boundary pathBoundary) bool {
	for _, arg := range args {
		lower := strings.ToLower(arg)
		if name == "xxd" && (hasShortOption(arg, 'r') || strings.HasPrefix(lower, "--revert")) {
			return false
		}
		if name == "wc" && (lower == "--files0-from" || strings.HasPrefix(lower, "--files0-from=")) {
			return false
		}
	}
	return allPathsInside(boundary, nonFlagArguments(args))
}

func safeDiffArguments(args []string, boundary pathBoundary) bool {
	for index, arg := range args {
		lower := strings.ToLower(arg)
		if lower == "-o" || strings.HasPrefix(lower, "-o") && !strings.HasPrefix(lower, "--") ||
			lower == "--output" || strings.HasPrefix(lower, "--output=") {
			return false
		}
		if optionPathInside(args, index, lower, "--from-file", boundary) == optionPathOutside ||
			optionPathInside(args, index, lower, "--to-file", boundary) == optionPathOutside {
			return false
		}
	}
	return allPathsInside(boundary, nonFlagArguments(args))
}

func safeSortArguments(args []string, boundary pathBoundary) bool {
	for index, arg := range args {
		lower := strings.ToLower(arg)
		if lower == "-o" || strings.HasPrefix(lower, "-o") && !strings.HasPrefix(lower, "--") ||
			lower == "--output" || strings.HasPrefix(lower, "--output=") ||
			arg == "-T" || strings.HasPrefix(arg, "-T") ||
			lower == "--temporary-directory" || strings.HasPrefix(lower, "--temporary-directory=") ||
			lower == "--compress-program" || strings.HasPrefix(lower, "--compress-program=") {
			return false
		}
		if optionPathInside(args, index, lower, "--random-source", boundary) == optionPathOutside {
			return false
		}
	}
	return allPathsInside(boundary, nonFlagArguments(args))
}

func safeFindArguments(args []string, boundary pathBoundary) bool {
	unsafe := map[string]bool{
		"-delete": true, "-exec": true, "-execdir": true, "-ok": true, "-okdir": true,
		"-fprint": true, "-fprint0": true, "-fprintf": true, "-fls": true,
		"-files0-from": true,
	}
	roots := make([]string, 0, 2)
	expressionStarted := false
	for index, arg := range args {
		lower := strings.ToLower(arg)
		if unsafe[lower] {
			return false
		}
		if arg == "-L" || arg == "-H" || lower == "-follow" {
			return false
		}
		if looksLikeExternalPath(arg) && !boundary.containsLiteral(arg) {
			return false
		}
		if oneOf(lower, "-newer", "-anewer", "-cnewer", "-samefile") ||
			strings.HasPrefix(lower, "-newer") && len(lower) > len("-newer") {
			if index+1 >= len(args) || !boundary.containsLiteral(args[index+1]) {
				return false
			}
		}
		if !expressionStarted && (strings.HasPrefix(arg, "-") || arg == "!" || arg == "(" || arg == ")") {
			expressionStarted = true
		}
		if !expressionStarted {
			roots = append(roots, arg)
		}
	}
	if len(roots) == 0 {
		roots = append(roots, ".")
	}
	return allPathsInside(boundary, roots)
}

func hasShortOption(argument string, option byte) bool {
	return len(argument) > 1 && argument[0] == '-' && argument[1] != '-' &&
		strings.ContainsRune(argument[1:], rune(option))
}
