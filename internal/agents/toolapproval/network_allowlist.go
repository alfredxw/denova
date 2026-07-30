package toolapproval

import "strings"

func safeCurl(args []string, boundary pathBoundary) bool {
	if hasExternalLiteralArgument(args, boundary) {
		return false
	}
	for index, arg := range args {
		lower := strings.ToLower(arg)
		if strings.Contains(lower, "://") && !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") &&
			!strings.HasPrefix(lower, "--url=http://") && !strings.HasPrefix(lower, "--url=https://") {
			return false
		}
		if arg == "-d" || arg == "-F" || arg == "-T" ||
			oneOf(lower, "--data", "--data-raw", "--data-binary", "--data-urlencode", "--form", "--upload-file") ||
			strings.HasPrefix(arg, "-d") || strings.HasPrefix(arg, "-F") || strings.HasPrefix(arg, "-T") ||
			strings.HasPrefix(lower, "--data=") || strings.HasPrefix(lower, "--form=") || strings.HasPrefix(lower, "--upload-file=") {
			return false
		}
		if arg == "-X" || lower == "--request" {
			if index+1 >= len(args) || !strings.EqualFold(args[index+1], "GET") {
				return false
			}
		}
		if strings.HasPrefix(arg, "-X") && arg != "-X" && !strings.EqualFold(strings.TrimPrefix(arg, "-X"), "GET") ||
			strings.HasPrefix(lower, "--request=") && !strings.EqualFold(strings.TrimPrefix(lower, "--request="), "get") {
			return false
		}
		if arg == "-K" || strings.HasPrefix(arg, "-K") ||
			lower == "--config" || strings.HasPrefix(lower, "--config=") ||
			arg == "-n" || lower == "--netrc" || lower == "--netrc-optional" {
			return false
		}
		if arg == "-w" || strings.HasPrefix(arg, "-w") ||
			lower == "--write-out" || strings.HasPrefix(lower, "--write-out=") ||
			lower == "--unix-socket" || strings.HasPrefix(lower, "--unix-socket=") ||
			lower == "--abstract-unix-socket" || strings.HasPrefix(lower, "--abstract-unix-socket=") ||
			credentialedNetworkOption(arg) ||
			strings.HasPrefix(arg, "-u") || strings.HasPrefix(arg, "-U") {
			return false
		}
		if (arg == "-H" || lower == "--header") && index+1 < len(args) && sensitiveHTTPHeader(args[index+1]) ||
			strings.HasPrefix(arg, "-H") && arg != "-H" && sensitiveHTTPHeader(strings.TrimPrefix(arg, "-H")) ||
			strings.HasPrefix(lower, "--header=") && sensitiveHTTPHeader(arg[len("--header="):]) {
			return false
		}
		if arg == "-o" || lower == "--output" {
			if index+1 >= len(args) || !boundary.containsLiteral(args[index+1]) {
				return false
			}
		}
		if strings.HasPrefix(arg, "-o") && arg != "-o" && !boundary.containsLiteral(strings.TrimPrefix(arg, "-o")) {
			return false
		}
		if strings.HasPrefix(lower, "--output=") && !boundary.containsLiteral(strings.TrimPrefix(arg, "--output=")) {
			return false
		}
		if oneOf(lower, "--output-dir", "--netrc-file", "--cookie", "--cookie-jar", "--cert", "--key",
			"--dump-header", "--trace", "--trace-ascii", "--stderr", "--alt-svc", "--hsts", "--etag-save") {
			if index+1 >= len(args) || !boundary.containsLiteral(args[index+1]) {
				return false
			}
		}
		if strings.HasPrefix(arg, "-D") && arg != "-D" && !boundary.containsLiteral(strings.TrimPrefix(arg, "-D")) ||
			strings.HasPrefix(arg, "-b") && arg != "-b" && looksLikeExternalPath(strings.TrimPrefix(arg, "-b")) &&
				!boundary.containsLiteral(strings.TrimPrefix(arg, "-b")) ||
			strings.HasPrefix(arg, "-c") && arg != "-c" && !boundary.containsLiteral(strings.TrimPrefix(arg, "-c")) {
			return false
		}
		for _, option := range []string{
			"--output-dir=", "--netrc-file=", "--cookie=", "--cookie-jar=", "--cert=", "--key=",
			"--dump-header=", "--trace=", "--trace-ascii=", "--stderr=", "--alt-svc=", "--hsts=", "--etag-save=",
		} {
			if strings.HasPrefix(lower, option) && !boundary.containsLiteral(arg[len(option):]) {
				return false
			}
		}
	}
	return len(args) > 0
}

func safeWget(args []string, boundary pathBoundary) bool {
	if hasExternalLiteralArgument(args, boundary) {
		return false
	}
	for index, arg := range args {
		lower := strings.ToLower(arg)
		if strings.Contains(lower, "://") && !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
			return false
		}
		if strings.HasPrefix(lower, "--post-") || strings.HasPrefix(lower, "--method=") && !strings.EqualFold(strings.TrimPrefix(lower, "--method="), "get") ||
			lower == "--body-data" || strings.HasPrefix(lower, "--body-data=") || lower == "--body-file" || strings.HasPrefix(lower, "--body-file=") {
			return false
		}
		if arg == "-e" || strings.HasPrefix(arg, "-e") ||
			lower == "--execute" || strings.HasPrefix(lower, "--execute=") ||
			lower == "--config" || strings.HasPrefix(lower, "--config=") ||
			lower == "--use-askpass" || strings.HasPrefix(lower, "--use-askpass=") ||
			credentialedNetworkOption(arg) && arg != "-U" {
			return false
		}
		if lower == "--header" && index+1 < len(args) && sensitiveHTTPHeader(args[index+1]) ||
			strings.HasPrefix(lower, "--header=") && sensitiveHTTPHeader(arg[len("--header="):]) {
			return false
		}
		if arg == "-O" || lower == "--output-document" || arg == "-P" || lower == "--directory-prefix" ||
			arg == "-i" || lower == "--input-file" ||
			arg == "-o" || lower == "--output-file" || arg == "-a" || lower == "--append-output" ||
			oneOf(lower, "--save-cookies", "--load-cookies", "--ca-certificate", "--certificate", "--private-key", "--warc-file", "--warc-tempdir") {
			if arg == "-i" || lower == "--input-file" {
				return false
			}
			if index+1 >= len(args) || !boundary.containsLiteral(args[index+1]) {
				return false
			}
		}
		if strings.HasPrefix(arg, "-O") && arg != "-O" && !boundary.containsLiteral(strings.TrimPrefix(arg, "-O")) ||
			strings.HasPrefix(arg, "-P") && arg != "-P" && !boundary.containsLiteral(strings.TrimPrefix(arg, "-P")) ||
			strings.HasPrefix(arg, "-o") && arg != "-o" && !boundary.containsLiteral(strings.TrimPrefix(arg, "-o")) ||
			strings.HasPrefix(arg, "-a") && arg != "-a" && !boundary.containsLiteral(strings.TrimPrefix(arg, "-a")) ||
			strings.HasPrefix(arg, "-i") && arg != "-i" {
			return false
		}
		if strings.HasPrefix(lower, "--output-document=") && !boundary.containsLiteral(strings.TrimPrefix(arg, "--output-document=")) {
			return false
		}
		if strings.HasPrefix(lower, "--directory-prefix=") && !boundary.containsLiteral(arg[len("--directory-prefix="):]) ||
			strings.HasPrefix(lower, "--input-file=") {
			return false
		}
		for _, option := range []string{
			"--output-file=", "--append-output=", "--save-cookies=", "--load-cookies=",
			"--ca-certificate=", "--certificate=", "--private-key=", "--warc-file=", "--warc-tempdir=",
		} {
			if strings.HasPrefix(lower, option) && !boundary.containsLiteral(arg[len(option):]) {
				return false
			}
		}
	}
	return len(args) > 0
}

func credentialedNetworkOption(argument string) bool {
	name, _, _ := strings.Cut(argument, "=")
	if name == "-u" || name == "-U" {
		return true
	}
	return oneOf(strings.ToLower(name),
		"--user", "--proxy-user", "--oauth2-bearer", "--pass",
		"--negotiate", "--ntlm", "--anyauth", "--basic", "--digest", "--aws-sigv4",
		"--http-user", "--http-password", "--proxy-password", "--password",
	)
}

func sensitiveHTTPHeader(value string) bool {
	value = strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(value, "authorization:") ||
		strings.HasPrefix(value, "proxy-authorization:") ||
		strings.HasPrefix(value, "cookie:")
}
