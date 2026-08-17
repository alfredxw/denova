package tools

import (
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/text/encoding/htmlindex"
)

type grepCommandDialect uint8

const (
	grepDialectRipgrep grepCommandDialect = iota
	grepDialectGrep
)

// normalizeGrepWords accepts a small, explicit compatibility surface before
// the strict rg compiler runs. It never executes a shell or guesses unknown
// options: every accepted grep spelling is translated to canonical rg argv.
func normalizeGrepWords(words []string) ([]string, bool, error) {
	if len(words) == 0 {
		return nil, false, grepCommandError("invalid_command", "grep command is empty")
	}
	result := append([]string(nil), words...)
	dialect := grepDialectRipgrep
	changed := false
	switch result[0] {
	case "rg":
	case "ripgrep":
		result[0], changed = "rg", true
	case "grep", "egrep":
		result[0], dialect, changed = "rg", grepDialectGrep, true
	case "fgrep":
		result[0], dialect, changed = "rg", grepDialectGrep, true
		result = append([]string{result[0], "-F"}, result[1:]...)
	default:
		return nil, false, grepCommandError(
			"invalid_command",
			fmt.Sprintf("command must use rg, not %q", result[0]),
		)
	}
	if dialect == grepDialectGrep {
		translated, err := translateGrepOptions(result)
		return translated, true, err
	}
	return normalizeRipgrepCarryover(result, changed)
}

var grepCompatibleLongOptions = map[string]bool{
	"after-context":       true,
	"before-context":      true,
	"context":             true,
	"count":               false,
	"regexp":              true,
	"fixed-strings":       false,
	"with-filename":       false,
	"ignore-case":         false,
	"files-with-matches":  false,
	"files-without-match": false,
	"max-count":           true,
	"line-number":         false,
	"only-matching":       false,
	"invert-match":        false,
	"word-regexp":         false,
	"line-regexp":         false,
}

func translateGrepOptions(words []string) ([]string, error) {
	result := []string{"rg"}
	optionsEnded := false
	for index := 1; index < len(words); index++ {
		token := words[index]
		if optionsEnded || token == "" || token == "-" || !strings.HasPrefix(token, "-") {
			result = append(result, token)
			continue
		}
		if token == "--" {
			optionsEnded = true
			result = append(result, token)
			continue
		}
		if strings.HasPrefix(token, "--") {
			translated, consumed, err := translateLongGrepOption(words, index)
			if err != nil {
				return nil, err
			}
			result = append(result, translated...)
			index += consumed - 1
			continue
		}
		translated, consumed, err := translateShortGrepOptions(words, index)
		if err != nil {
			return nil, err
		}
		result = append(result, translated...)
		index += consumed - 1
	}
	return result, nil
}

func translateLongGrepOption(words []string, index int) ([]string, int, error) {
	token := words[index]
	name, attachedValue, attached := strings.Cut(strings.TrimPrefix(token, "--"), "=")
	if takesValue, ok := grepCompatibleLongOptions[name]; ok {
		if !takesValue {
			if attached {
				return nil, 0, grepCompatibilityError(token, "remove the unexpected option value")
			}
			return []string{"--" + name}, 1, nil
		}
		value, consumed, err := grepOptionValue(words, index, name, attachedValue, attached)
		if err != nil {
			return nil, 0, err
		}
		return []string{"--" + name, value}, consumed, nil
	}
	switch name {
	case "extended-regexp", "recursive", "dereference-recursive", "line-buffered", "no-filename":
		if attached {
			return nil, 0, grepCompatibilityError(token, "remove the unexpected option value")
		}
		return nil, 1, nil
	case "color":
		return nil, 1, nil
	case "perl-regexp":
		if attached {
			return nil, 0, grepCompatibilityError(token, "use rg --pcre2")
		}
		return []string{"--pcre2"}, 1, nil
	case "include", "exclude", "exclude-dir":
		value, consumed, err := grepOptionValue(words, index, name, attachedValue, attached)
		if err != nil {
			return nil, 0, err
		}
		switch name {
		case "include":
			return []string{"-g", value}, consumed, nil
		case "exclude":
			return []string{"-g", ensureNegativeGlob(value)}, consumed, nil
		default:
			return []string{"-g", excludedDirectoryGlob(value)}, consumed, nil
		}
	case "quiet":
		return nil, 0, grepCompatibilityError(token, "use rg -l when only matching file paths are needed")
	case "file":
		return nil, 0, grepCompatibilityError(token, "use repeated rg -e patterns instead of a pattern file")
	case "basic-regexp":
		return nil, 0, grepCompatibilityError(token, "rewrite the pattern for rg's default regular expression syntax")
	default:
		return nil, 0, grepCompatibilityError(token, "use the corresponding supported rg option")
	}
}

func translateShortGrepOptions(words []string, index int) ([]string, int, error) {
	token := words[index]
	body := strings.TrimPrefix(token, "-")
	if body == "" {
		return nil, 0, grepCompatibilityError(token, "use rg -e '-' to search for a dash")
	}
	result := make([]string, 0, len(body))
	consumed := 1
	for offset := 0; offset < len(body); offset++ {
		name := body[offset]
		switch name {
		case 'F', 'H', 'P', 'c', 'i', 'l', 'n', 'o', 'v', 'w', 'x':
			result = append(result, "-"+string(name))
		case 'A', 'B', 'C', 'e', 'm':
			value := body[offset+1:]
			if value == "" {
				if index+1 >= len(words) || words[index+1] == "--" {
					return nil, 0, grepCompatibilityError("-"+string(name), "provide its required value")
				}
				value, consumed = words[index+1], 2
			}
			result = append(result, "-"+string(name), value)
			offset = len(body)
		case 'E', 'I', 'R', 'h', 'r', 's':
			// rg already provides recursive extended-regex search, binary-file
			// skipping and controlled diagnostics/output framing.
		case 'L':
			result = append(result, "--files-without-match")
		case 'q':
			return nil, 0, grepCompatibilityError("-q", "use rg -l when only matching file paths are needed")
		case 'f':
			return nil, 0, grepCompatibilityError("-f", "use repeated rg -e patterns instead of a pattern file")
		case 'G':
			return nil, 0, grepCompatibilityError("-G", "rewrite the pattern for rg's default regular expression syntax")
		default:
			return nil, 0, grepCompatibilityError("-"+string(name), "use the corresponding supported rg option")
		}
	}
	return result, consumed, nil
}

func grepOptionValue(words []string, index int, name, attachedValue string, attached bool) (string, int, error) {
	if attached {
		if attachedValue == "" {
			return "", 0, grepCompatibilityError("--"+name, "provide its required value")
		}
		return attachedValue, 1, nil
	}
	if index+1 >= len(words) || words[index+1] == "--" {
		return "", 0, grepCompatibilityError("--"+name, "provide its required value")
	}
	return words[index+1], 2, nil
}

func ensureNegativeGlob(value string) string {
	if strings.HasPrefix(value, "!") {
		return value
	}
	return "!" + value
}

func excludedDirectoryGlob(value string) string {
	value = strings.TrimSuffix(value, "/")
	if !strings.Contains(value, "/") {
		value = "**/" + value
	}
	return ensureNegativeGlob(value + "/**")
}

func grepCompatibilityError(option, suggestion string) error {
	return grepCommandError(
		"unsupported_flag",
		fmt.Sprintf("grep option %s cannot be translated safely; canonical rg: %s", option, suggestion),
	)
}

func normalizeRipgrepCarryover(words []string, changed bool) ([]string, bool, error) {
	result := make([]string, 0, len(words))
	optionsEnded := false
	for index := 0; index < len(words); index++ {
		token := words[index]
		if index == 0 || optionsEnded || token == "" || token == "-" || !strings.HasPrefix(token, "-") {
			result = append(result, token)
			continue
		}
		if token == "--" {
			optionsEnded = true
			result = append(result, token)
			continue
		}
		if strings.HasPrefix(token, "--") {
			result = append(result, token)
			if ripgrepLongOptionConsumesNext(token) && index+1 < len(words) {
				index++
				result = append(result, words[index])
			}
			continue
		}
		next := ""
		if index+1 < len(words) {
			next = words[index+1]
		}
		normalized, normalizedToken := normalizeRipgrepShortCarryover(token, next)
		changed = changed || normalizedToken
		if normalized != "" {
			result = append(result, normalized)
		}
		if ripgrepShortOptionConsumesNext(normalized) && index+1 < len(words) {
			index++
			result = append(result, words[index])
		}
	}
	return result, changed, nil
}

func normalizeRipgrepShortCarryover(token, next string) (string, bool) {
	body := strings.TrimPrefix(token, "-")
	if body == "" {
		return token, false
	}
	normalized, changed := removeLikelyRecursiveFlag(body)
	if len(normalized) > 1 && strings.HasSuffix(normalized, "E") &&
		isNoValueGrepCluster(strings.TrimSuffix(normalized, "E")) && !isRipgrepEncoding(next) {
		normalized = strings.TrimSuffix(normalized, "E")
		changed = true
	}
	if !changed {
		return token, false
	}
	if normalized == "" {
		return "", true
	}
	return "-" + normalized, true
}

func removeLikelyRecursiveFlag(body string) (string, bool) {
	if body == "R" {
		return "", true
	}
	for index, name := range []byte(body) {
		if name != 'R' && name != 'r' {
			continue
		}
		remainder := body[:index] + body[index+1:]
		if remainder != "" && isNoValueGrepCluster(remainder) {
			return remainder, true
		}
	}
	return body, false
}

func isNoValueGrepCluster(value string) bool {
	if value == "" {
		return false
	}
	for _, name := range []byte(value) {
		switch name {
		case '.', 'F', 'H', 'P', 'S', 'U', 'a', 'c', 'i', 'l', 'n', 'o', 's', 'u', 'v', 'w', 'x':
		default:
			return false
		}
	}
	return true
}

func isRipgrepEncoding(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "auto", "none":
		return true
	case "":
		return false
	}
	_, err := htmlindex.Get(value)
	return err == nil
}

func ripgrepLongOptionConsumesNext(token string) bool {
	nameValue := strings.TrimPrefix(token, "--")
	name, _, attached := strings.Cut(nameValue, "=")
	if attached {
		return false
	}
	if spec, exists := grepLongFlagSpecs[name]; exists {
		return spec.value
	}
	return false
}

func ripgrepShortOptionConsumesNext(token string) bool {
	body := strings.TrimPrefix(token, "-")
	for offset := 0; offset < len(body); offset++ {
		name := body[offset]
		spec, exists := grepShortFlagSpecs[name]
		if !exists || !spec.value {
			continue
		}
		return offset+1 == len(body)
	}
	return false
}

func canonicalGrepCommand(command compiledGrepCommand) string {
	stages := make([]string, 0, len(command.stages))
	for _, stage := range command.stages {
		words := append([]string{"rg"}, stage.args...)
		words = append(words, "--")
		words = append(words, stage.paths...)
		for index, word := range words {
			words[index] = quoteCanonicalGrepWord(word)
		}
		stages = append(stages, strings.Join(words, " "))
	}
	return strings.Join(stages, " | ")
}

func quoteCanonicalGrepWord(value string) string {
	if value != "" {
		safe := true
		for _, current := range value {
			if unicode.IsLetter(current) || unicode.IsDigit(current) || strings.ContainsRune("_./:=+,-", current) {
				continue
			}
			safe = false
			break
		}
		if safe {
			return value
		}
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
