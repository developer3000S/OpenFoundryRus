package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var defaultServices = []string{
	"pipeline-build-service",
	"notebook-runtime-service",
	"ontology-actions-service",
	"authorization-policy-service",
	"federation-product-exchange-service",
	"ontology-indexer",
}

var methodNames = []string{"get", "post", "put", "patch", "delete", "head", "options"}

var goMethods = map[string]string{
	"Get": "GET", "Post": "POST", "Put": "PUT", "Patch": "PATCH", "Delete": "DELETE", "Head": "HEAD", "Options": "OPTIONS",
}

type Route struct {
	Service string
	Side    string
	Method  string
	Path    string
	Handler string
	File    string
	Line    int
	Status  string
}

type rustBlock struct {
	Name   string
	Start  int
	End    int
	Merges []string
}

func normalizePath(path string) string {
	path = regexp.MustCompile(`\{([^}:]+):[^}]+\}`).ReplaceAllString(path, `{$1}`)
	path = regexp.MustCompile(`<([^>]+)>`).ReplaceAllString(path, `{$1}`)
	path = regexp.MustCompile(`/+`).ReplaceAllString(path, `/`)
	if path != "/" {
		path = strings.TrimRight(path, "/")
	}
	if path == "" {
		return "/"
	}
	return path
}

func joinPaths(prefix, path string) string {
	if prefix == "" {
		return normalizePath(path)
	}
	return normalizePath(strings.TrimRight(prefix, "/") + "/" + strings.TrimLeft(path, "/"))
}

func comparablePath(path string) string {
	return regexp.MustCompile(`\{[^}]+\}`).ReplaceAllString(normalizePath(path), "{}")
}

func rel(path, root string) string {
	if r, err := filepath.Rel(root, path); err == nil {
		return filepath.ToSlash(r)
	}
	return filepath.ToSlash(path)
}

func lineNo(text string, idx int) int { return strings.Count(text[:idx], "\n") + 1 }

func findMatching(text string, openIdx int, openCh, closeCh byte) int {
	depth := 0
	inString, escaped := false, false
	inLineComment := false
	inBlockComment := false
	for i := openIdx; i < len(text); i++ {
		ch := text[i]
		next := byte(0)
		if i+1 < len(text) {
			next = text[i+1]
		}
		if inLineComment {
			if ch == '\n' {
				inLineComment = false
			}
			continue
		}
		if inBlockComment {
			if ch == '*' && next == '/' {
				inBlockComment = false
				i++
			}
			continue
		}
		if inString {
			if escaped {
				escaped = false
			} else if ch == '\\' {
				escaped = true
			} else if ch == '"' {
				inString = false
			}
			continue
		}
		if ch == '/' && next == '/' {
			inLineComment = true
			i++
			continue
		}
		if ch == '/' && next == '*' {
			inBlockComment = true
			i++
			continue
		}
		if ch == '"' {
			inString = true
			continue
		}
		if ch == openCh {
			depth++
		}
		if ch == closeCh {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func findStatementEnd(text string, start int) int {
	inString, escaped, inLineComment := false, false, false
	depth := 0
	for i := start; i < len(text); i++ {
		ch := text[i]
		next := byte(0)
		if i+1 < len(text) {
			next = text[i+1]
		}
		if inLineComment {
			if ch == '\n' {
				inLineComment = false
			}
			continue
		}
		if inString {
			if escaped {
				escaped = false
			} else if ch == '\\' {
				escaped = true
			} else if ch == '"' {
				inString = false
			}
			continue
		}
		if ch == '/' && next == '/' {
			inLineComment = true
			i++
			continue
		}
		if ch == '"' {
			inString = true
			continue
		}
		switch ch {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			if depth > 0 {
				depth--
			}
		case ';':
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func splitTopLevelArgs(text string) []string {
	var args []string
	start, depth := 0, 0
	inString, escaped := false, false
	for i := 0; i < len(text); i++ {
		ch := text[i]
		if inString {
			if escaped {
				escaped = false
			} else if ch == '\\' {
				escaped = true
			} else if ch == '"' {
				inString = false
			}
			continue
		}
		if ch == '"' {
			inString = true
			continue
		}
		switch ch {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case ',':
			if depth == 0 {
				args = append(args, strings.TrimSpace(text[start:i]))
				start = i + 1
			}
		}
	}
	if tail := strings.TrimSpace(text[start:]); tail != "" {
		args = append(args, tail)
	}
	return args
}

func iterFiles(root, suffix string) []string {
	var files []string
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, suffix) && !strings.HasSuffix(path, "_test"+suffix) {
			files = append(files, path)
		}
		return nil
	})
	sort.Strings(files)
	return files
}

func rustBlocks(text string) []rustBlock {
	letRe := regexp.MustCompile(`let\s+(\w+)\s*=\s*`)
	mergeRe := regexp.MustCompile(`(?:^|\b|\.merge\s*\()([A-Za-z_]\w*)\b`)
	reserved := map[string]bool{"Router": true, "new": true, "route": true, "merge": true, "nest": true, "layer": true, "with_state": true, "get": true, "post": true, "put": true, "patch": true, "delete": true}
	var blocks []rustBlock
	for _, m := range letRe.FindAllStringSubmatchIndex(text, -1) {
		end := findStatementEnd(text, m[1])
		if end == -1 {
			continue
		}
		name := text[m[2]:m[3]]
		expr := text[m[1]:end]
		var merges []string
		seen := map[string]bool{}
		for _, mm := range mergeRe.FindAllStringSubmatchIndex(expr, -1) {
			v := expr[mm[2]:mm[3]]
			if v != name && !reserved[v] && !seen[v] {
				merges = append(merges, v)
				seen[v] = true
			}
		}
		blocks = append(blocks, rustBlock{Name: name, Start: m[0], End: end, Merges: merges})
	}
	return blocks
}

func rustPrefixes(text string, blocks []rustBlock) map[string]string {
	prefixes := map[string]string{}
	nestRe := regexp.MustCompile(`\.nest\s*\(\s*"([^"]+)"\s*,\s*(\w+)\s*\)`)
	for _, m := range nestRe.FindAllStringSubmatch(text, -1) {
		prefixes[m[2]] = joinPaths(prefixes[m[2]], m[1])
	}
	changed := true
	for changed {
		changed = false
		for _, b := range blocks {
			p := prefixes[b.Name]
			if p == "" {
				continue
			}
			for _, child := range b.Merges {
				if prefixes[child] == "" {
					prefixes[child] = p
					changed = true
				}
			}
		}
	}
	return prefixes
}

func blockAt(blocks []rustBlock, idx int) string {
	best := ""
	bestSize := int(^uint(0) >> 1)
	for _, b := range blocks {
		if b.Start <= idx && idx <= b.End && b.End-b.Start < bestSize {
			best, bestSize = b.Name, b.End-b.Start
		}
	}
	return best
}

func dedupeRoutes(routes []Route) []Route {
	seen := map[string]bool{}
	out := make([]Route, 0, len(routes))
	for _, r := range routes {
		key := r.Side + " " + r.Method + " " + comparablePath(r.Path)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, r)
	}
	return out
}

func extractRustRoutes(repo, service string) []Route {
	root := filepath.Join(repo, "services", service, "src")
	var routes []Route
	for _, file := range iterFiles(root, ".rs") {
		data, _ := os.ReadFile(file)
		text := string(data)
		blocks := rustBlocks(text)
		prefixes := rustPrefixes(text, blocks)
		for _, m := range regexp.MustCompile(`\.route\s*\(`).FindAllStringIndex(text, -1) {
			open := strings.IndexByte(text[m[0]:], '(')
			if open < 0 {
				continue
			}
			open += m[0]
			close := findMatching(text, open, '(', ')')
			if close == -1 {
				continue
			}
			args := splitTopLevelArgs(text[open+1 : close])
			if len(args) < 2 {
				continue
			}
			pm := regexp.MustCompile(`^"([^"]+)"`).FindStringSubmatch(strings.TrimSpace(args[0]))
			if pm == nil {
				continue
			}
			full := joinPaths(prefixes[blockAt(blocks, m[0])], pm[1])
			expr := args[1]
			found := false
			for _, method := range methodNames {
				re := regexp.MustCompile(`\b` + method + `\s*\(\s*([^\)]+?)\s*\)`)
				for _, hm := range re.FindAllStringSubmatch(expr, -1) {
					h := strings.TrimSpace(hm[1])
					routes = append(routes, Route{service, "rust", strings.ToUpper(method), full, h, rel(file, repo), lineNo(text, m[0]), "unknown"})
					found = true
				}
			}
			if !found {
				routes = append(routes, Route{service, "rust", "ANY", full, strings.TrimSpace(expr), rel(file, repo), lineNo(text, m[0]), "unknown"})
			}
		}
	}
	return dedupeRoutes(routes)
}

func extractGoFunctionBodies(root string) map[string][]string {
	bodies := map[string][]string{}
	fnRe := regexp.MustCompile(`func\s+(?:\([^\)]*\)\s*)?(\w+)\s*\(`)
	for _, file := range iterFiles(root, ".go") {
		data, _ := os.ReadFile(file)
		text := string(data)
		for _, m := range fnRe.FindAllStringSubmatchIndex(text, -1) {
			brace := strings.IndexByte(text[m[1]:], '{')
			if brace < 0 {
				continue
			}
			brace += m[1]
			close := findMatching(text, brace, '{', '}')
			if close != -1 {
				bodies[text[m[2]:m[3]]] = append(bodies[text[m[2]:m[3]]], text[brace+1:close])
			}
		}
	}
	return bodies
}

func classifyGoHandler(handler string, bodies map[string][]string) string {
	name := handler[strings.LastIndex(handler, ".")+1:]
	candidates := bodies[name]
	if len(candidates) == 0 {
		return "implemented"
	}
	sawEmpty, sawConfig := false, false
	for _, body := range candidates {
		compact := regexp.MustCompile(`\s+`).ReplaceAllString(body, " ")
		lower := strings.ToLower(body)
		if strings.Contains(body, "http.StatusNotImplemented") || strings.Contains(body, "notImplemented(w") || (strings.Contains(body, "StatusServiceUnavailable") && strings.Contains(lower, "pending")) {
			return "501"
		}
		if strings.Contains(body, "writeEmptyList") || strings.Contains(compact, `"data": []any{}`) || strings.Contains(body, "[]any{}") {
			sawEmpty = true
		}
		if strings.Contains(lower, "disabled") || strings.Contains(body, "StatusServiceUnavailable") || strings.Contains(body, "StatusBadGateway") || strings.Contains(body, "PythonSidecarBinary") || strings.Contains(body, "if m != nil") {
			sawConfig = true
		}
	}
	if sawEmpty {
		return "empty-envelope"
	}
	if sawConfig {
		return "config-gated"
	}
	return "implemented"
}

func extractGoRoutes(repo, service string) []Route {
	root := filepath.Join(repo, "openfoundry-go", "services", service)
	bodies := extractGoFunctionBodies(root)
	var routes []Route
	routeStart := regexp.MustCompile(`(\w+)\.Route\s*\(\s*"([^"]+)"\s*,\s*func\s*\(\s*(\w+)\s+chi\.Router\s*\)`)
	direct := regexp.MustCompile(`(\w+)\.(Get|Post|Put|Patch|Delete|Head|Options)\s*\(\s*"([^"]+)"\s*,\s*([^\)\s,]+)`)
	methodCall := regexp.MustCompile(`(\w+)\.Method\s*\(\s*http\.Method(\w+)\s*,\s*"([^"]+)"\s*,\s*([^\)\s,]+)`)
	type frame struct {
		child string
		depth int
	}
	for _, file := range iterFiles(root, ".go") {
		data, _ := os.ReadFile(file)
		text := string(data)
		prefix := map[string]string{"r": ""}
		var stack []frame
		for no, line := range strings.Split(text, "\n") {
			lineNo := no + 1
			if m := routeStart.FindStringSubmatch(line); m != nil {
				prefix[m[3]] = joinPaths(prefix[m[1]], m[2])
				stack = append(stack, frame{m[3], strings.Count(line, "{") - strings.Count(line, "}")})
			}
			for _, m := range direct.FindAllStringSubmatch(line, -1) {
				full := joinPaths(prefix[m[1]], m[3])
				h := m[4]
				routes = append(routes, Route{service, "go", goMethods[m[2]], full, h, rel(file, repo), lineNo, classifyGoHandler(h, bodies)})
			}
			if m := methodCall.FindStringSubmatch(line); m != nil {
				full := joinPaths(prefix[m[1]], m[3])
				h := m[4]
				routes = append(routes, Route{service, "go", strings.ToUpper(m[2]), full, h, rel(file, repo), lineNo, classifyGoHandler(h, bodies)})
			}
			if len(stack) > 0 {
				top := &stack[len(stack)-1]
				top.depth += strings.Count(line, "{") - strings.Count(line, "}")
				if top.depth <= 0 {
					delete(prefix, top.child)
					stack = stack[:len(stack)-1]
				}
			}
		}
	}
	return dedupeRoutes(routes)
}

func routeKey(r Route) string { return r.Method + " " + comparablePath(r.Path) }

func reportForService(repo, service string) string {
	rust, goRoutes := extractRustRoutes(repo, service), extractGoRoutes(repo, service)
	rb, gb := map[string]Route{}, map[string]Route{}
	for _, r := range rust {
		rb[routeKey(r)] = r
	}
	for _, g := range goRoutes {
		gb[routeKey(g)] = g
	}
	keySet := map[string]bool{}
	for k := range rb {
		keySet[k] = true
	}
	for k := range gb {
		keySet[k] = true
	}
	var keys []string
	for k := range keySet {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		pi := strings.SplitN(keys[i], " ", 2)
		pj := strings.SplitN(keys[j], " ", 2)
		if pi[1] == pj[1] {
			return pi[0] < pj[0]
		}
		return pi[1] < pj[1]
	})
	counts := map[string]int{}
	var rows []string
	for _, k := range keys {
		rr, rok := rb[k]
		gg, gok := gb[k]
		status := "missing"
		if gok {
			status = gg.Status
		}
		counts[status]++
		displayPath := strings.SplitN(k, " ", 2)[1]
		if rok {
			displayPath = rr.Path
		} else if gok {
			displayPath = gg.Path
		}
		rustHandler := "—"
		if rok {
			rustHandler = fmt.Sprintf("`%s`<br><sub>%s:%d</sub>", rr.Handler, rr.File, rr.Line)
		}
		goHandler := "—"
		if gok {
			goHandler = fmt.Sprintf("`%s`<br><sub>%s:%d</sub>", gg.Handler, gg.File, gg.Line)
		}
		method := strings.SplitN(k, " ", 2)[0]
		rows = append(rows, fmt.Sprintf("| `%s` | %s | %s | %s | %s |", displayPath, method, rustHandler, goHandler, status))
	}
	var countKeys []string
	for k := range counts {
		countKeys = append(countKeys, k)
	}
	sort.Strings(countKeys)
	var countParts []string
	for _, k := range countKeys {
		countParts = append(countParts, fmt.Sprintf("%s: %d", k, counts[k]))
	}
	lines := []string{fmt.Sprintf("## %s", service), "", fmt.Sprintf("Rust routes: %d. Go routes: %d.", len(rust), len(goRoutes)), "State counts: " + strings.Join(countParts, ", ") + ".", "", "| Path | Method | Rust handler | Go handler | Status |", "| --- | --- | --- | --- | --- |"}
	lines = append(lines, rows...)
	lines = append(lines, "")
	return strings.Join(lines, "\n")
}

func generateReport(repo string, services []string) string {
	parts := []string{"# Rust vs Go route parity audit", "", "Date: " + time.Now().UTC().Format("2006-01-02"), "", "Generated with:", "", "```sh", "cd openfoundry-go && go run ./tools/route-audit --write docs/migration/route-parity-audit.md", "```", "", "Status values:", "", "- `implemented`: route exists in Go and the Go handler is not detected as a placeholder.", "- `missing`: Rust route was not found in Go.", "- `501`: Go route exists but its handler advertises Not Implemented or pending behavior.", "- `empty-envelope`: Go route exists but returns an empty/list placeholder envelope.", "- `config-gated`: Go route exists but operation depends on optional runtime/config wiring.", "", "This report is generated by a heuristic parser for repository-local Axum and chi route declarations. It compares path structure while ignoring URL parameter names.", ""}
	for _, s := range services {
		parts = append(parts, reportForService(repo, s))
	}
	return strings.TrimRight(strings.Join(parts, "\n"), "\n") + "\n"
}

func findRepoRoot(start string) string {
	cur, _ := filepath.Abs(start)
	for {
		if _, err := os.Stat(filepath.Join(cur, "openfoundry-go", "go.mod")); err == nil {
			return cur
		}
		if filepath.Base(cur) == "openfoundry-go" {
			if _, err := os.Stat(filepath.Join(filepath.Dir(cur), "services")); err == nil {
				return filepath.Dir(cur)
			}
		}
		next := filepath.Dir(cur)
		if next == cur {
			return start
		}
		cur = next
	}
}

func main() {
	write := flag.String("write", "", "write report to path")
	repoFlag := flag.String("repo", "", "repository root containing services and openfoundry-go")
	servicesFlag := flag.String("services", strings.Join(defaultServices, ","), "comma-separated service list")
	flag.Parse()
	repo := *repoFlag
	if repo == "" {
		repo = findRepoRoot(".")
	}
	var services []string
	for _, s := range strings.Split(*servicesFlag, ",") {
		if strings.TrimSpace(s) != "" {
			services = append(services, strings.TrimSpace(s))
		}
	}
	report := generateReport(repo, services)
	if *write != "" {
		if err := os.MkdirAll(filepath.Dir(*write), 0o755); err != nil {
			panic(err)
		}
		if err := os.WriteFile(*write, []byte(report), 0o644); err != nil {
			panic(err)
		}
		return
	}
	fmt.Print(report)
}
