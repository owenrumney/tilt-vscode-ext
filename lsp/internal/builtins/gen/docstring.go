package main

import (
	"regexp"
	"strings"
)

const docBaseURL = "https://api.tilt.dev/"

var (
	// `text <url.html>`_ and `text <https://...>`_
	linkRe = regexp.MustCompile("`([^`<]+?)\\s*<([^>]+)>`_+")
	// :class:`~api.Link`, :meth:`k8s_resource`, :data:`x`
	roleRe = regexp.MustCompile(":[a-z]+:`~?(?:api\\.)?([^`]+)`")
	// ``literal`` becomes `literal`; done before single-backtick cleanup.
	literalRe = regexp.MustCompile("``([^`]+)``")
	// A bare `word` in reST is an interpreted-text reference, not code.
	indentRe = regexp.MustCompile(`(?m)^[ \t]+`)
)

// splitDocstring separates the prose from the Args: block, returning the prose
// as markdown and a per-parameter map, also markdown.
func splitDocstring(raw string) (string, map[string]string) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	lines := strings.Split(dedent(raw), "\n")

	argsAt := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "Args:" {
			argsAt = i
			break
		}
	}
	if argsAt < 0 {
		return toMarkdown(strings.Join(lines, "\n")), nil
	}
	prose := toMarkdown(strings.Join(lines[:argsAt], "\n"))
	return prose, parseArgs(lines[argsAt+1:])
}

// parseArgs reads "name: text" entries, joining indented continuation lines.
func parseArgs(lines []string) map[string]string {
	out := map[string]string{}
	name := ""
	var buf []string
	flush := func() {
		if name != "" {
			out[name] = toMarkdown(strings.Join(buf, " "))
		}
		name, buf = "", nil
	}
	entry := regexp.MustCompile(`^(\s*)([A-Za-z_][A-Za-z0-9_]*):\s?(.*)$`)
	baseIndent := -1

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		m := entry.FindStringSubmatch(line)
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		if m != nil && (baseIndent < 0 || indent <= baseIndent) {
			flush()
			baseIndent = indent
			name = m[2]
			if m[3] != "" {
				buf = append(buf, strings.TrimSpace(m[3]))
			}
			continue
		}
		if name != "" {
			buf = append(buf, strings.TrimSpace(line))
		}
	}
	flush()
	return out
}

// toMarkdown converts the handful of reST constructs the stubs actually use.
// Anything else passes through, which is better than mangling it.
func toMarkdown(text string) string {
	out := literalRe.ReplaceAllString(text, "`$1`")
	out = linkRe.ReplaceAllStringFunc(out, func(m string) string {
		parts := linkRe.FindStringSubmatch(m)
		return "[" + strings.TrimSpace(parts[1]) + "](" + absolute(parts[2]) + ")"
	})
	out = roleRe.ReplaceAllString(out, "`$1`")
	return strings.TrimSpace(collapseBlankLines(out))
}

// absolute resolves a relative doc link, which would otherwise be dead in a
// hover popup.
func absolute(url string) string {
	if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		return url
	}
	return docBaseURL + strings.TrimPrefix(url, "/")
}

// dedent removes the common leading whitespace a Python docstring carries.
func dedent(text string) string {
	lines := strings.Split(text, "\n")
	common := -1
	for i, line := range lines {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue
		}
		n := len(indentRe.FindString(line))
		if common < 0 || n < common {
			common = n
		}
	}
	if common <= 0 {
		return text
	}
	for i, line := range lines {
		if i == 0 {
			continue
		}
		if len(line) >= common {
			lines[i] = line[common:]
		} else {
			lines[i] = strings.TrimLeft(line, " \t")
		}
	}
	return strings.Join(lines, "\n")
}

func collapseBlankLines(text string) string {
	for strings.Contains(text, "\n\n\n") {
		text = strings.ReplaceAll(text, "\n\n\n", "\n\n")
	}
	return text
}
