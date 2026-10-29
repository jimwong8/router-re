// guard_race_rw: static guard against "load whole + save whole" without a lock.
//
// usage:
//
//	go run scripts/guard_race_rw.go <pkg-dir> [whitelist.json]
//
// It walks <pkg-dir> for *.go (including _test.go), and for every FuncDecl
// whose body contains BOTH a receiver read (s.load*(...)) and a receiver write
// (s.save*(...)) reports it UNLESS the same body also contains a Lock/Unlock
// (incl. RLock/RUnlock) call. This catches the «读整份 → 改一条 → 写整份»
// handler that forgets to take the guard mutex, so the race surfaces in CI
// instead of only under real concurrency.
//
// whitelist.json maps "file:line" or "file:func" -> reason. Un-whitelisted
// hits make the tool exit 1.
package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	// Methods on *Server that read the store: loadJSON, loadVoucher, loadDDNS, ...
	isRead = regexp.MustCompile(`^[lL]oad[a-zA-Z0-9_]*$`)
	// Methods on *Server that write the store: saveVoucher, saveDDNS, ...
	isWrite = regexp.MustCompile(`^[sS]ave[a-zA-Z0-9_]*$`)
	// Any mutex guard.
	isLock = map[string]bool{"Lock": true, "Unlock": true, "RLock": true, "RUnlock": true}
)

type Hit struct {
	File   string `json:"file"`
	Line   int    `json:"line"`
	Func   string `json:"func"`
	Recv   string `json:"recv"`
	Reads  string `json:"reads"`  // comma-sep load calls seen
	Writes string `json:"writes"` // comma-sep save calls seen
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: guard_race_rw <pkg-dir> [whitelist.json]")
		os.Exit(2)
	}
	pkgDir := os.Args[1]
	whitelist := map[string]string{}
	if len(os.Args) >= 3 {
		b, err := os.ReadFile(os.Args[2])
		if err != nil {
			fmt.Fprintf(os.Stderr, "read whitelist: %v\n", err)
			os.Exit(2)
		}
		if err := json.Unmarshal(b, &whitelist); err != nil {
			fmt.Fprintf(os.Stderr, "parse whitelist: %v\n", err)
			os.Exit(2)
		}
	}

	fset := token.NewFileSet()
	var hits []Hit
	err := filepath.WalkDir(pkgDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		f, err := parser.ParseFile(fset, path, src, parser.ParseComments)
		if err != nil {
			return nil
		}
		ast.Inspect(f, func(n ast.Node) bool {
			fn, ok := n.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				return true
			}
			recv := ""
			if fn.Recv != nil && len(fn.Recv.List) > 0 {
				fl := fn.Recv.List[0]
				if len(fl.Names) > 0 {
					recv = fl.Names[0].Name // receiver variable, e.g. "s"
				}
			}
			var reads, writes []string
			hasLock := false
			ast.Inspect(fn.Body, func(nn ast.Node) bool {
				ce, ok := nn.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := ce.Fun.(*ast.SelectorExpr)
				if ok {
					// guard check: any Lock/Unlock call = guarded.
					if isLock[sel.Sel.Name] && !hasLock {
						hasLock = true
					}
					// receiver read/write: <recvIdent>.loadX / <recvIdent>.saveY
					if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == recv && recv != "" {
						if isRead.MatchString(sel.Sel.Name) {
							reads = append(reads, sel.Sel.Name)
						} else if isWrite.MatchString(sel.Sel.Name) {
							writes = append(writes, sel.Sel.Name)
						}
					}
				}
				return true
			})
			if len(reads) > 0 && len(writes) > 0 && !hasLock {
				hits = append(hits, Hit{
					File:   relPath(path, pkgDir),
					Line:   fset.Position(fn.Pos()).Line,
					Func:   fn.Name.Name,
					Recv:   recv,
					Reads:  joinDedup(reads),
					Writes: joinDedup(writes),
				})
			}
			return true
		})
		return nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "walk: %v\n", err)
		os.Exit(2)
	}

	sort.Slice(hits, func(i, j int) bool {
		if hits[i].File != hits[j].File {
			return hits[i].File < hits[j].File
		}
		return hits[i].Line < hits[j].Line
	})

	var unwhitelisted []Hit
	printed := 0
	for _, h := range hits {
		key1 := fmt.Sprintf("%s:%d", h.File, h.Line)
		key2 := fmt.Sprintf("%s:%s", h.File, h.Func)
		reason, ok := whitelist[key1]
		if !ok {
			reason, ok = whitelist[key2]
		}
		if ok {
			fmt.Printf("  (whitelisted) %s:%d %s[%s] reads=%s writes=%s — %s\n",
				h.File, h.Line, h.Func, h.Recv, h.Reads, h.Writes, reason)
			printed++
			continue
		}
		fmt.Printf("  RACE-PRONE %s:%d %s[%s] reads=%s writes=%s (no Lock/Unlock in body)\n",
			h.File, h.Line, h.Func, h.Recv, h.Reads, h.Writes)
		unwhitelisted = append(unwhitelisted, h)
		printed++
	}
	fmt.Printf("\nScanned %s: %d load+save functions examined.\n", pkgDir, printed)
	if len(unwhitelisted) > 0 {
		fmt.Printf("FAIL: %d load+save funcs unlocked/not-whitelisted:\n", len(unwhitelisted))
		for _, h := range unwhitelisted {
			fmt.Printf("   - %s:%d %s\n", h.File, h.Line, h.Func)
		}
		os.Exit(1)
	}
	fmt.Println("PASS: all load+save functions are guarded by a lock (or whitelisted).")
}

func relPath(p, base string) string {
	r, err := filepath.Rel(base, p)
	if err != nil {
		return p
	}
	return r
}

func joinDedup(in []string) string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return strings.Join(out, ",")
}
