package main

// robots.txt, read as data and nothing else.
//
// Two kinds of group bind the catalogue reader: everyone's (*), and any group
// that names a crawler gathering pages for AI. A site that bars GPTBot has
// said what it thinks of its pages being collected for a model, and Claude
// drafts from what we collect, so a block in any of those groups stops us
// (memory: harvest-respects-ai-crawler-blocks). Within a group the longest
// matching rule decides, and Allow wins a tie, as Google and RFC 9309 read it.

import "strings"

var robotsAgents = map[string]bool{"*": true, "claudebot": true, "claude-web": true, "anthropic-ai": true, "gptbot": true, "ccbot": true}

type robotsRule struct {
	allow   bool
	pattern string
}

// robots holds only the groups that bind us.
type robots struct{ groups [][]robotsRule }

func parseRobots(body []byte) robots {
	var out robots
	var agents []string
	var rules []robotsRule
	inRules := false
	flush := func() {
		for _, a := range agents {
			if robotsAgents[a] {
				out.groups = append(out.groups, rules)
				break
			}
		}
		agents, rules, inRules = nil, nil, false
	}
	for _, line := range strings.Split(string(body), "\n") {
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key, value = strings.ToLower(strings.TrimSpace(key)), strings.TrimSpace(value)
		switch key {
		case "user-agent":
			if inRules {
				flush()
			}
			agents = append(agents, strings.ToLower(value))
		case "allow", "disallow":
			inRules = true
			if value != "" {
				rules = append(rules, robotsRule{allow: key == "allow", pattern: value})
			}
		}
	}
	flush()
	return out
}

// allowed reports whether every binding group lets us fetch path (with its query).
func (r robots) allowed(path string) bool {
	for _, g := range r.groups {
		best, allow := -1, true
		for _, rule := range g {
			if !robotsMatch(rule.pattern, path) {
				continue
			}
			if n := len(rule.pattern); n > best || (n == best && rule.allow) {
				best, allow = n, rule.allow
			}
		}
		if !allow {
			return false
		}
	}
	return true
}

// robotsMatch matches a robots pattern: * is any run of characters, a final $
// anchors the end, and otherwise a pattern is a prefix.
func robotsMatch(pattern, path string) bool {
	anchored := strings.HasSuffix(pattern, "$")
	pattern = strings.TrimSuffix(pattern, "$")
	parts := strings.Split(pattern, "*")
	if !strings.HasPrefix(path, parts[0]) {
		return false
	}
	pos := len(parts[0])
	for i, part := range parts[1:] {
		if anchored && i == len(parts)-2 {
			return strings.HasSuffix(path[pos:], part)
		}
		j := strings.Index(path[pos:], part)
		if j < 0 {
			return false
		}
		pos += j + len(part)
	}
	return !anchored || pos == len(path)
}
