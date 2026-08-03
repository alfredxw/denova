package toolapproval

import "strings"

func safePrintfArguments(args []string) bool {
	if len(args) == 0 {
		return true
	}
	if args[0] == "--" {
		args = args[1:]
	} else if args[0] == "-v" || strings.HasPrefix(args[0], "-v") {
		// Bash printf -v and the %n conversion mutate shell variables. The
		// static analyzer intentionally has no hidden state channel for either.
		return false
	}
	if len(args) == 0 {
		return true
	}
	format := args[0]
	for index := 0; index < len(format); index++ {
		if format[index] != '%' || index+1 >= len(format) {
			continue
		}
		if format[index+1] == '%' {
			index++
			continue
		}
		for conversion := index + 1; conversion < len(format); conversion++ {
			character := format[conversion]
			if character == 'n' {
				return false
			}
			if character >= 'A' && character <= 'Z' || character >= 'a' && character <= 'z' {
				break
			}
		}
	}
	return true
}

func safeAWKReadArguments(args []string, boundary pathBoundary) bool {
	program, inputs, ok := splitAWKArguments(args)
	if !ok || !safeAWKProgram(program) {
		return false
	}
	for _, input := range inputs {
		if input == "-" || awkAssignmentArgument(input) {
			continue
		}
		if !boundary.containsLiteral(input) {
			return false
		}
	}
	return true
}

func splitAWKArguments(args []string) (string, []string, bool) {
	flagsEnded := false
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if !flagsEnded && argument == "--" {
			flagsEnded = true
			continue
		}
		if !flagsEnded && strings.HasPrefix(argument, "-") && argument != "-" {
			lower := strings.ToLower(argument)
			switch {
			case argument == "-F" || lower == "--field-separator":
				if index+1 >= len(args) {
					return "", nil, false
				}
				index++
				continue
			case strings.HasPrefix(argument, "-F") && argument != "-F" || strings.HasPrefix(lower, "--field-separator="):
				continue
			case argument == "-v" || lower == "--assign":
				if index+1 >= len(args) || !awkAssignmentArgument(args[index+1]) {
					return "", nil, false
				}
				index++
				continue
			case strings.HasPrefix(argument, "-v") && argument != "-v":
				if !awkAssignmentArgument(strings.TrimPrefix(argument, "-v")) {
					return "", nil, false
				}
				continue
			case strings.HasPrefix(lower, "--assign="):
				if !awkAssignmentArgument(argument[len("--assign="):]) {
					return "", nil, false
				}
				continue
			case oneOf(lower, "--posix", "--traditional", "--lint", "--sandbox"):
				continue
			case argument == "-W":
				if index+1 >= len(args) || !safeAWKCompatibilityOption(args[index+1]) {
					return "", nil, false
				}
				index++
				continue
			case strings.HasPrefix(argument, "-W") && argument != "-W":
				if !safeAWKCompatibilityOption(strings.TrimPrefix(argument, "-W")) {
					return "", nil, false
				}
				continue
			default:
				// Program files, extension libraries, and unknown implementation
				// options may execute code that is absent from this command string.
				return "", nil, false
			}
		}
		return argument, args[index+1:], true
	}
	return "", nil, false
}

func safeAWKCompatibilityOption(value string) bool {
	value, _, _ = strings.Cut(strings.ToLower(strings.TrimSpace(value)), "=")
	return oneOf(value, "posix", "traditional", "lint", "sandbox")
}

func awkAssignmentArgument(value string) bool {
	name, _, found := strings.Cut(value, "=")
	if !found || name == "" || !awkIdentifierStart(name[0]) {
		return false
	}
	for index := 1; index < len(name); index++ {
		if !awkIdentifierPart(name[index]) {
			return false
		}
	}
	return true
}

func safeAWKProgram(program string) bool {
	quoted := byte(0)
	escaped := false
	for index := 0; index < len(program); {
		character := program[index]
		if quoted != 0 {
			if escaped {
				escaped = false
				index++
				continue
			}
			if character == '\\' {
				escaped = true
				index++
				continue
			}
			if character == quoted {
				quoted = 0
			}
			index++
			continue
		}
		switch character {
		case '"':
			quoted = character
			index++
			continue
		case '#', '>', '|', '@':
			// Redirection, process pipes, gawk source extensions, and the
			// comment-or-regex ambiguity of '#' require full AWK parsing. Prompting
			// for these programs is safer than guessing their grammar.
			return false
		case '\\':
			if index+1 < len(program) && program[index+1] == '\n' {
				return false
			}
		}
		if awkIdentifierStart(character) {
			start := index
			index++
			for index < len(program) && awkIdentifierPart(program[index]) {
				index++
			}
			switch strings.ToLower(program[start:index]) {
			case "system", "getline", "argv", "argc", "environ":
				return false
			}
			continue
		}
		index++
	}
	return quoted == 0 && !escaped
}

func awkIdentifierStart(character byte) bool {
	return character == '_' || character >= 'A' && character <= 'Z' || character >= 'a' && character <= 'z'
}

func awkIdentifierPart(character byte) bool {
	return awkIdentifierStart(character) || character >= '0' && character <= '9'
}
