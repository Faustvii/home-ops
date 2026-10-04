// Command konflate prints konflate's rendered Flux diff for one pull request as
// plain text, shaped for an agent's run tool: konflate's summary first, then
// every non-CRD resource as a unified diff, then CRDs condensed to the schema
// paths they add, remove or change. The agent never parses JSON or HTML.
//
//	konflate --server https://konflate.example.com <pr>             the overview
//	konflate --server https://konflate.example.com <pr> <resource>  one resource in full
//
// <resource> is an id from the overview (r3) or a substring of a title. The run
// tool hands commands no environment beyond PATH, HOME and the proxy, so the
// server is a flag (KONFLATE_URL works too, for local use).
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
	// CA roots for when the host has none (a scratch image run on its own);
	// the system pool still wins where one exists, as in the runner.
	_ "golang.org/x/crypto/x509roots/fallback"
)

const (
	// The run tool keeps 32 KiB of a command's output; stay under it so the
	// closing notes are never the part that is cut.
	maxOutput = 28 << 10
	// Per-section budgets in the overview, so one huge resource cannot crowd
	// the others out.
	maxResource = 8 << 10
	maxCRD      = 5 << 10
	userAgent   = "kritika-konflate/1.0" // Cloudflare's integrity check rejects Go's default
)

type row struct {
	Hunk   bool   `json:"hunk"`
	Folded bool   `json:"folded"`
	Count  int    `json:"count"`
	Kind   string `json:"kind"`
	HTML   string `json:"html"`
}

type resource struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Kind    string `json:"kind"`
	Parent  string `json:"parent"`
	Status  string `json:"status"`
	Add     int    `json:"add"`
	Del     int    `json:"del"`
	Unified []row  `json:"unified"`
}

type diffEnvelope struct {
	Status string `json:"status"`
	Error  string `json:"error"`
	Diff   *struct {
		HeadSha   string     `json:"headSha"`
		Truncated int        `json:"truncated"`
		Resources []resource `json:"resources"`
	} `json:"diff"`
}

var errPending = errors.New("pending")

// self is how this command was invoked, server included, so the hints it
// prints can be run as they are.
var self string

func main() {
	server := flag.String("server", os.Getenv("KONFLATE_URL"), "konflate base URL")
	wait := flag.Duration("wait", 25*time.Second, "give up after this long (the run tool stops commands at 30s)")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: konflate --server URL <pr> [resource id or title substring]")
	}
	flag.Parse()
	if *server == "" || flag.NArg() < 1 || flag.NArg() > 2 {
		flag.Usage()
		os.Exit(2)
	}
	pr, err := strconv.Atoi(flag.Arg(0))
	if err != nil || pr <= 0 {
		fail("pull request number %q is not a positive integer", flag.Arg(0))
	}
	self = "konflate --server " + *server
	ctx, cancel := context.WithTimeout(context.Background(), *wait)
	defer cancel()
	c := client{base: strings.TrimRight(*server, "/"), http: &http.Client{}}

	env, err := c.diff(ctx, pr)
	switch {
	case errors.Is(err, errPending) || (err != nil && ctx.Err() != nil):
		fmt.Printf("konflate is still rendering PR #%d. Run the same command again in a minute.\n", pr)
		return
	case err != nil:
		fail("%v", err)
	case env == nil:
		fmt.Printf("konflate does not track PR #%d (forks and PRs outside its filter are not rendered). Review without it.\n", pr)
		return
	case env.Diff == nil:
		fmt.Printf("konflate could not render PR #%d: %s\n", pr, cmpOr(oneLine(env.Error), env.Status))
		return
	}

	out := &budget{limit: maxOutput}
	if flag.NArg() == 2 {
		writeOne(out, pr, env.Diff.Resources, flag.Arg(1))
	} else {
		summary, err := c.summary(ctx, pr)
		if err != nil {
			summary = fmt.Sprintf("(konflate summary unavailable: %v)", err)
		}
		writeOverview(out, pr, env, summary)
	}
	os.Stdout.WriteString(out.String())
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "konflate: "+format+"\n", args...)
	os.Exit(1)
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// --- konflate API ---

type client struct {
	base string
	http *http.Client
}

func (c client) get(ctx context.Context, path, accept string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", accept)
	return c.http.Do(req)
}

// diff fetches the rendered diff, polling while konflate renders. A nil
// envelope with a nil error means konflate does not track the PR.
func (c client) diff(ctx context.Context, pr int) (*diffEnvelope, error) {
	for {
		resp, err := c.get(ctx, fmt.Sprintf("/api/prs/%d/diff", pr), "application/json")
		if err != nil {
			return nil, err
		}
		switch resp.StatusCode {
		case http.StatusOK:
			var env diffEnvelope
			err := json.NewDecoder(resp.Body).Decode(&env)
			resp.Body.Close()
			if err != nil {
				return nil, fmt.Errorf("decoding the diff: %w", err)
			}
			return &env, nil
		case http.StatusNotFound:
			resp.Body.Close()
			return nil, nil
		case http.StatusAccepted:
			resp.Body.Close()
		default:
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
			resp.Body.Close()
			return nil, fmt.Errorf("GET diff: HTTP %d: %s", resp.StatusCode, oneLine(string(body)))
		}
		select {
		case <-ctx.Done():
			return nil, errPending
		case <-time.After(5 * time.Second):
		}
	}
}

func (c client) summary(ctx context.Context, pr int) (string, error) {
	resp, err := c.get(ctx, fmt.Sprintf("/api/prs/%d/summary", pr), "text/markdown")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	// Drop the HTML marker comment konflate uses to find its own PR comment.
	s := regexp.MustCompile(`(?m)^<!--.*-->\n?`).ReplaceAllString(string(body), "")
	return strings.TrimSpace(s), nil
}

// --- output ---

// budget is a builder that stops accepting text past its limit and remembers
// that it did, so the caller can say what was left out.
type budget struct {
	strings.Builder
	limit int
	full  bool
}

func (b *budget) add(s string) bool {
	if b.full {
		return false
	}
	if b.Len()+len(s) > b.limit {
		b.full = true
		return false
	}
	b.WriteString(s)
	return true
}

// note writes past the limit; maxOutput leaves room under the run tool's cap
// for the closing notes.
func (b *budget) note(s string) { b.WriteString(s) }

func writeOverview(out *budget, pr int, env *diffEnvelope, summary string) {
	d := env.Diff
	var crds, others []resource
	for _, r := range d.Resources {
		if r.Kind == "CustomResourceDefinition" {
			crds = append(crds, r)
		} else {
			others = append(others, r)
		}
	}

	head := d.HeadSha
	if len(head) > 7 {
		head = head[:7]
	}
	out.add(fmt.Sprintf("konflate: PR #%d rendered by flate at head %s against its merge-base. "+
		"This is the Kubernetes YAML Flux would apply after Helm and kustomize, not the git diff.\n\n", pr, head))
	out.add("## Summary\n\n" + summary + "\n\n")

	out.add("## Changed resources\n\n")
	if len(d.Resources) == 0 {
		out.add("(none: the change renders to the same manifests)\n")
	}
	for _, r := range d.Resources {
		out.add(fmt.Sprintf("%-4s %-8s %s (+%d -%d) via %s\n", r.ID, r.Status, r.Title, r.Add, r.Del, r.Parent))
	}
	if d.Truncated > 0 {
		out.add(fmt.Sprintf("… and %d more changed resources konflate did not render (its resource cap).\n", d.Truncated))
	}

	var skipped []string
	if len(others) > 0 {
		out.add("\n## Diffs\n")
		for i, r := range others {
			sec := &budget{limit: maxResource}
			writeUnified(sec, r)
			text := sec.String()
			if sec.full {
				text += fmt.Sprintf("    … cut here; run `%s %d %s` for all of it …\n", self, pr, r.ID)
			}
			if !out.add("\n" + text) {
				for _, r := range others[i:] {
					skipped = append(skipped, r.ID)
				}
				break
			}
		}
	}

	if len(crds) > 0 && out.full {
		for _, r := range crds {
			skipped = append(skipped, r.ID)
		}
	} else if len(crds) > 0 {
		out.add("\n## CRD schema changes\n\n" +
			"Schema paths only: + added, - removed, ~ value changed. Description-only edits are counted, not shown. " +
			"Added subtrees are listed by their root.\n")
		for i, r := range crds {
			sec := &budget{limit: maxCRD}
			writeCRD(sec, r)
			text := sec.String()
			if sec.full {
				text += fmt.Sprintf("    … cut here; run `%s %d %s` for the full YAML diff …\n", self, pr, r.ID)
			}
			if !out.add("\n" + text) {
				for _, r := range crds[i:] {
					skipped = append(skipped, r.ID)
				}
				break
			}
		}
	}

	if len(skipped) > 0 {
		out.note(fmt.Sprintf("\n… output cut at the size limit before %s; fetch each with `%s %d <id>` …\n",
			strings.Join(skipped, ", "), self, pr))
	}
}

func writeOne(out *budget, pr int, rs []resource, q string) {
	ql := strings.ToLower(q)
	var ids []string
	matched := false
	for _, r := range rs {
		ids = append(ids, r.ID+" "+r.Title)
		if r.ID != q && !strings.Contains(strings.ToLower(r.Title), ql) {
			continue
		}
		matched = true
		writeUnified(out, r)
		out.add("\n")
	}
	if !matched {
		out.add(fmt.Sprintf("No resource in PR #%d matches %q. Resources:\n%s\n", pr, q, strings.Join(ids, "\n")))
	}
	if out.full {
		out.note("\n… output cut at the size limit …\n")
	}
}

var htmlTag = regexp.MustCompile(`<[^>]*>`)

// text turns one chroma-highlighted row back into plain text. chroma escapes
// every token, so a raw '<' only ever opens a tag.
func text(r row) string { return html.UnescapeString(htmlTag.ReplaceAllString(r.HTML, "")) }

func prefix(kind string) string {
	switch kind {
	case "add":
		return "+"
	case "del":
		return "-"
	}
	return " "
}

// writeUnified renders a resource the way konflate's own plain-text view
// does: collapsed context becomes a count, changed lines keep three lines of
// context, which konflate's rows already carry.
func writeUnified(b *budget, r resource) {
	b.add(fmt.Sprintf("### %s %s (%s, +%d -%d)\n", r.ID, r.Title, r.Status, r.Add, r.Del))
	for _, x := range r.Unified {
		var line string
		switch {
		case x.Folded:
			continue
		case x.Hunk:
			line = fmt.Sprintf("    … %d unchanged lines …\n", x.Count)
		default:
			line = prefix(x.Kind) + text(x) + "\n"
		}
		if !b.add(line) {
			return
		}
	}
}

// --- CRD condensing ---

// writeCRD compares a CRD's two sides structurally instead of by line: a line
// diff of a large schema pairs look-alike lines (every `type: string`) across
// unrelated fields. konflate's rows carry the whole document on both sides,
// folded context included, so each side is rebuilt and parsed.
func writeCRD(b *budget, r resource) {
	b.add(fmt.Sprintf("### %s %s (%s, +%d -%d)\n", r.ID, r.Title, r.Status, r.Add, r.Del))
	var oldDoc, newDoc strings.Builder
	for _, x := range r.Unified {
		if x.Hunk {
			continue
		}
		t := text(x) + "\n"
		if x.Kind != "add" {
			oldDoc.WriteString(t)
		}
		if x.Kind != "del" {
			newDoc.WriteString(t)
		}
	}
	var before, after any
	if err := yaml.Unmarshal([]byte(oldDoc.String()), &before); err != nil {
		b.add(fmt.Sprintf("  (could not parse the old side: %v; fetch the resource for the raw diff)\n", err))
		return
	}
	if err := yaml.Unmarshal([]byte(newDoc.String()), &after); err != nil {
		b.add(fmt.Sprintf("  (could not parse the new side: %v; fetch the resource for the raw diff)\n", err))
		return
	}
	d := &differ{}
	// Labels and annotations on a CRD are chart bookkeeping, not schema.
	for _, doc := range []any{before, after} {
		if m, ok := doc.(map[string]any); ok {
			delete(m, "metadata")
			delete(m, "status")
		}
	}
	d.walk(nil, before, after)
	for _, l := range d.lines {
		if !b.add(l + "\n") {
			return
		}
	}
	if d.descriptions > 0 {
		b.add(fmt.Sprintf("  (%d description-only edits)\n", d.descriptions))
	}
	if len(d.lines) == 0 && d.descriptions == 0 {
		b.add("  (no schema changes)\n")
	}
}

type differ struct {
	lines        []string
	descriptions int
}

func (d *differ) walk(path []string, a, b any) {
	if len(path) > 0 && path[len(path)-1] == "description" {
		if !equal(a, b) {
			d.descriptions++
		}
		return
	}
	switch av := a.(type) {
	case map[string]any:
		bv, ok := b.(map[string]any)
		if !ok {
			d.changed(path, a, b)
			return
		}
		for _, k := range sortedKeys(av, bv) {
			x, inA := av[k]
			y, inB := bv[k]
			p := append(slicesClone(path), k)
			switch {
			case !inA:
				d.add('+', p, y)
			case !inB:
				d.add('-', p, x)
			default:
				d.walk(p, x, y)
			}
		}
	case []any:
		bv, ok := b.([]any)
		if !ok {
			d.changed(path, a, b)
			return
		}
		d.list(path, av, bv)
	default:
		if !equal(a, b) {
			d.changed(path, a, b)
		}
	}
}

// list compares a sequence. Scalar lists (required, enum) are compared as
// sets; lists of mappings are matched by an identifying key when they have
// one, else by position.
func (d *differ) list(path []string, a, b []any) {
	if allScalars(a) && allScalars(b) {
		for _, x := range a {
			if !containsValue(b, x) {
				d.lines = append(d.lines, fmt.Sprintf("- %s[]: %s", render(path), short(x)))
			}
		}
		for _, y := range b {
			if !containsValue(a, y) {
				d.lines = append(d.lines, fmt.Sprintf("+ %s[]: %s", render(path), short(y)))
			}
		}
		return
	}
	if key := identity(a, b); key != "" {
		index := func(l []any) (map[string]any, []string) {
			m, order := map[string]any{}, []string{}
			for _, x := range l {
				id := fmt.Sprint(x.(map[string]any)[key])
				m[id] = x
				order = append(order, id)
			}
			return m, order
		}
		am, aOrder := index(a)
		bm, bOrder := index(b)
		seg := func(id string) []string {
			p := slicesClone(path)
			p[len(p)-1] += "[" + truncate(id, 60) + "]"
			return p
		}
		for _, id := range aOrder {
			if _, ok := bm[id]; !ok {
				d.add('-', seg(id), am[id])
			}
		}
		for _, id := range bOrder {
			if x, ok := am[id]; ok {
				d.walk(seg(id), x, bm[id])
			} else {
				d.add('+', seg(id), bm[id])
			}
		}
		return
	}
	n := min(len(a), len(b))
	for i := 0; i < n; i++ {
		p := slicesClone(path)
		p[len(p)-1] += fmt.Sprintf("[%d]", i)
		d.walk(p, a[i], b[i])
	}
	for i := n; i < len(b); i++ {
		p := slicesClone(path)
		p[len(p)-1] += fmt.Sprintf("[%d]", i)
		d.add('+', p, b[i])
	}
	for i := n; i < len(a); i++ {
		p := slicesClone(path)
		p[len(p)-1] += fmt.Sprintf("[%d]", i)
		d.add('-', p, a[i])
	}
}

// add reports a key or item present on one side only: a scalar with its
// value, a subtree by its root.
func (d *differ) add(sign byte, path []string, v any) {
	if len(path) > 0 && path[len(path)-1] == "description" {
		d.descriptions++
		return
	}
	switch v.(type) {
	case map[string]any, []any:
		d.lines = append(d.lines, fmt.Sprintf("%c %s", sign, render(path)))
	default:
		d.lines = append(d.lines, fmt.Sprintf("%c %s: %s", sign, render(path), short(v)))
	}
}

func (d *differ) changed(path []string, a, b any) {
	d.lines = append(d.lines, fmt.Sprintf("~ %s: %s -> %s", render(path), short(a), short(b)))
}

// identity picks the key that names the mappings of both lists, if every
// item has one and the values are unique: name for versions and fields,
// jsonPath for printer columns, rule for CEL validations.
func identity(a, b []any) string {
	for _, key := range []string{"name", "jsonPath", "rule"} {
		ok := true
		for _, l := range [][]any{a, b} {
			seen := map[string]bool{}
			for _, x := range l {
				m, isMap := x.(map[string]any)
				v, has := m[key]
				id := fmt.Sprint(v)
				if !isMap || !has || seen[id] {
					ok = false
					break
				}
				seen[id] = true
			}
		}
		if ok {
			return key
		}
	}
	return ""
}

// render folds a schema path's scaffolding away: spec, schema and
// openAPIV3Schema go, properties disappears, items becomes [].
func render(path []string) string {
	var out []string
	field := false // the segment after properties names a field, even "items"
	for i, p := range path {
		switch {
		case field:
			out = append(out, p)
			field = false
		case i == 0 && p == "spec":
		case p == "schema" || p == "openAPIV3Schema":
		case p == "properties" && i+1 < len(path):
			field = true
		case p == "items" && i+1 < len(path):
			if len(out) > 0 {
				out[len(out)-1] += "[]"
			}
		default:
			out = append(out, p)
		}
	}
	return strings.Join(out, ".")
}

func sortedKeys(a, b map[string]any) []string {
	seen := map[string]bool{}
	var keys []string
	for _, m := range []map[string]any{a, b} {
		for k := range m {
			if !seen[k] {
				seen[k] = true
				keys = append(keys, k)
			}
		}
	}
	sort.Strings(keys)
	return keys
}

func allScalars(l []any) bool {
	for _, x := range l {
		switch x.(type) {
		case map[string]any, []any:
			return false
		}
	}
	return true
}

func containsValue(l []any, v any) bool {
	for _, x := range l {
		if equal(x, v) {
			return true
		}
	}
	return false
}

func equal(a, b any) bool { return reflect.DeepEqual(a, b) }

func short(v any) string {
	if v == nil {
		return "null"
	}
	return truncate(oneLine(fmt.Sprint(v)), 100)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func slicesClone(p []string) []string { return append([]string(nil), p...) }
