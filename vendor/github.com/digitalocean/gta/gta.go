/*
Copyright 2016 The gta AUTHORS. All rights reserved.

Use of this source code is governed by the Apache 2 license that can be found
in the LICENSE file.
*/
package gta

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/build"
	"go/scanner"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

var (
	// ErrNoDiffer is returned when there is no differ set on the GTA.
	ErrNoDiffer = errors.New("there is no differ set")
	// ErrNoPackager is returned when there is no packager set on the GTA.
	ErrNoPackager = errors.New("there is no packager set")
)

// Packages contains various detailed information about the structure of
// packages GTA has detected.
type Packages struct {
	// Dependencies contains a map of changed packages to their dependencies
	Dependencies map[string][]Package

	// Changes represents the changed files
	Changes []Package

	// AllChanges represents all packages that are dirty including the initial
	// changed packages.
	AllChanges []Package
}

type packagesJSON struct {
	Dependencies map[string][]string `json:"dependencies,omitempty"`
	Changes      []string            `json:"changes,omitempty"`
	AllChanges   []string            `json:"all_changes,omitempty"`
}

// MarshalJSON implements the json.Marshaler interface.
func (p *Packages) MarshalJSON() ([]byte, error) {
	s := packagesJSON{
		Dependencies: mapify(p.Dependencies),
		Changes:      stringify(p.Changes),
		AllChanges:   stringify(p.AllChanges),
	}
	return json.Marshal(s)
}

// UnmarshalJSON used by gtartifacts when providing a changed package list
// see `useChangedPackagesFrom()`
func (p *Packages) UnmarshalJSON(b []byte) error {
	s := new(packagesJSON)

	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}

	p.Dependencies = make(map[string][]Package)
	for k, v := range s.Dependencies {
		for _, vv := range v {
			p.Dependencies[k] = append(p.Dependencies[k], Package{ImportPath: vv})
		}
	}

	for _, v := range s.Changes {
		p.Changes = append(p.Changes, Package{ImportPath: v})
	}

	for _, v := range s.AllChanges {
		p.AllChanges = append(p.AllChanges, Package{ImportPath: v})
	}

	return nil
}

// A GTA provides a method of building dirty packages, and their dependent
// packages.
type GTA struct {
	differ   Differ
	packager Packager
	prefixes []string
	tags     []string
}

// New returns a new GTA with various options passed to New. Options will be
// applied in order so that later options can override earlier options.
func New(opts ...Option) (*GTA, error) {
	gta := &GTA{
		differ: NewGitDiffer(),
	}

	for _, opt := range opts {
		err := opt(gta)
		if err != nil {
			return nil, err
		}
	}

	// set the default packager after applying option so that the default
	// packager implementation does not load packages unnecessarily when the
	// packager is provided as an option.
	if gta.packager == nil {
		/*
			patterns, err := patternsFrom(gta.differ, gta.prefixes)
			if err != nil {
				return nil, err
			}

			gta.packager = NewPackager(patterns, gta.tags)
		*/

		// Cause NewPackager to return a packager that loads all packages by
		// passing a nil pattern.  This is important to ensure that all packages
		// are loaded and that nothing is skipped based on build tag constraints
		// when a file is changed. e.g. if a vendored file that is constrained to
		// Windows is changed, that package wouldn't load at all and trying to find
		// the package's dependencies would fail.
		gta.packager = NewPackager(nil, gta.tags)
	}

	return gta, nil
}

// ChangedPackages uses the differ and packager to build a map of changed root
// packages to their dependent packages where dependent is defined as "changed"
// as well due to their dependency to the changed packages. It returns the
// dependency graph, the changes differ detected and a set of all unique
// packages (including the changes).
//
// As an example: package "foo" is imported by packages "bar" and "qux". If
// "foo" has changed, it has two dependent packages, "bar" and "qux". The
// result would be then:
//
//   Dependencies = {"foo": ["bar", "qux"]}
//   Changes      = ["foo"]
//   AllChanges   = ["foo", "bar", "qux]
//
// Note that two different changed package might have the same dependent
// package. Below you see that both "foo" and "foo2" has changed. Each have
// "bar" because "bar" imports both "foo" and "foo2", i.e:
//
//   Dependencies = {"foo": ["bar", "qux"], "foo2" : ["afa", "bar", "qux"]}
//   Changes      = ["foo", "foo2"]
//   AllChanges   = ["foo", "foo2", "afa", "bar", "qux]
func (g *GTA) ChangedPackages() (*Packages, error) {
	paths, err := g.markedPackages()
	if err != nil {
		return nil, err
	}

	cp := &Packages{
		Dependencies: map[string][]Package{},
	}

	packageFromImport := func(path string) (*Package, error) {
		pkg, err := g.packager.PackageFromImport(path)
		if err != nil {
			return nil, err
		}

		return pkg, nil
	}

	// build our packages
	allChanges := map[string]Package{}
	for changed, marked := range paths {
		var packages []Package

		// add any dependents of the changed package; the changed package will be included in marked.
		for path, check := range marked {
			pkg := new(Package)
			pkg.ImportPath = path

			if check {
				pkg2, err := packageFromImport(path)
				if err != nil {
					return nil, err
				}
				pkg = pkg2
			}

			addPackage := func(pkg Package) {
				allChanges[pkg.ImportPath] = pkg
				if changed == pkg.ImportPath {
					cp.Changes = append(cp.Changes, pkg)
				} else {
					packages = append(packages, pkg)
				}
			}

			if hasPrefixIn(pkg.ImportPath, g.prefixes) {
				addPackage(*pkg)
			}
		}

		if len(packages) != 0 {
			sort.Sort(byPackageImportPath(packages))
			cp.Dependencies[changed] = packages
		}
	}

	for _, pkg := range allChanges {
		cp.AllChanges = append(cp.AllChanges, pkg)
	}
	sort.Sort(byPackageImportPath(cp.AllChanges))
	sort.Sort(byPackageImportPath(cp.Changes))

	return cp, nil
}

// markedPackages returns a map of maps. The outer map's key is the import path
// of a package that was changed according to g.differ. The inner maps' (i.e.
// the values of the outer map) keys are import paths of the dependents of the
// packages in respective key of the outer map. The inner maps' boolean values
// are true when the respective package exists and false when the respective
// package was deleted.
func (g *GTA) markedPackages() (map[string]map[string]bool, error) {
	if g.differ == nil {
		return nil, ErrNoDiffer
	}
	if g.packager == nil {
		return nil, ErrNoPackager
	}

	// get our diff'd directories
	dirs, err := g.differ.Diff()
	if err != nil {
		return nil, fmt.Errorf("diffing directory for dirty packages, %v", err)
	}

	// we build our set of initial dirty packages from the git diff. The map
	// value is true when the package was deleted.
	changed := make(map[string]bool)
	for abs, dir := range dirs {
		// TODO(bc): handle changes to go.mod when vendoring is not being used.

		// ignore deleted directories that contained no go files.
		// TODO(bc): make sure it was not within a testdata directory.
		if !dir.Exists && !hasGoFile(dir.Files) {
			continue
		}

		// Avoid .foo, _foo, and testdata directory trees how the go tool does!
		// See https://github.com/golang/tools/blob/3a85b8d/go/buildutil/allpackages.go#L93
		// Above link is not guaranteed to work.
		base := filepath.Base(abs)
		parent := filepath.Base(filepath.Dir(abs))
		// TODO(bc): do not ignore testdata directories - use their parent instead.
		if base == "" || base[0] == '.' || base[0] == '_' || base == "testdata" || parent == "testdata" {
			continue
		}

		pkg, err := g.packager.PackageFromDir(abs)
		if err != nil {
			switch err.(type) {
			case *build.NoGoError:
				if hasGoFile(dir.Files) {
					importPath, err := g.findImportPath(abs)
					if err != nil {
						continue
					}
					pkg.ImportPath = importPath

					changed[pkg.ImportPath] = true
					continue
				}
				// there are and were no buildable go files in this directory
				// so no dirty packages
				continue
			case scanner.ErrorList:
				// same, package is not buildable, so no dirty packages
				continue
			default:
				if !dir.Exists && hasGoFile(dir.Files) {
					importPath, err := g.findImportPath(abs)
					if err != nil {
						continue
					}
					changed[importPath] = true
					continue
				}
			}
			return nil, fmt.Errorf("pulling package information for %q, %v", abs, err)
		}

		// create a simple set of changed pkgs by import path
		changed[pkg.ImportPath] = false
	}

	// we build the dependent graph
	graph, err := g.packager.DependentGraph()
	if err != nil {
		return nil, fmt.Errorf("building dependency graph, %v", err)
	}

	paths := map[string]map[string]bool{}
	for change := range changed {
		marked := make(map[string]bool)

		// we traverse the graph and build our list of mark all dependents
		graph.Traverse(change, marked)

		// clear the boolean value on the paths that no longer contain packages (i.e.
		// the Go files were deleted...).
		for importPath := range marked {
			if changed[importPath] {
				marked[importPath] = false
			}
		}

		paths[change] = marked
	}

	return paths, nil
}

var errImportPathNotFound = errors.New("could not find import path")

// findImportPath walks a directory up, trying to find an import path for
// parent directories.
func (g *GTA) findImportPath(abs string) (string, error) {
	base := filepath.Base(abs)
	parent := filepath.Dir(abs)

	if base == abs {
		return "", errImportPathNotFound
	}

	if !exists(abs) {
		//	recurse when the directory doesn't exist
		importPath, err := g.findImportPath(parent)
		if err != nil && err == errImportPathNotFound {
			return path.Join(importPath, base), err
		}
		return path.Join(importPath, base), nil
	}

	pkg, err := g.packager.PackageFromDir(abs)
	if err != nil {
		if _, ok := err.(*build.NoGoError); ok {
			pkg, err := g.packager.PackageFromEmptyDir(abs)
			if err == nil {
				return pkg.ImportPath, nil
			}
		}
		importPath, err := g.findImportPath(parent)
		return path.Join(importPath, base), err
	}

	return pkg.ImportPath, nil
}

type byPackageImportPath []Package

func (b byPackageImportPath) Len() int               { return len(b) }
func (b byPackageImportPath) Less(i int, j int) bool { return b[i].ImportPath < b[j].ImportPath }
func (b byPackageImportPath) Swap(i int, j int)      { b[i], b[j] = b[j], b[i] }

func stringify(pkgs []Package) []string {
	var out []string
	for _, pkg := range pkgs {
		out = append(out, pkg.ImportPath)
	}
	return out
}

func mapify(pkgs map[string][]Package) map[string][]string {
	out := map[string][]string{}
	for key, pkgs := range pkgs {
		out[key] = stringify(pkgs)
	}
	return out
}

func hasGoFile(files []string) bool {
	for _, fn := range files {
		if filepath.Ext(fn) == ".go" {
			return true
		}
	}
	return false
}

func hasPrefixIn(s string, prefixes []string) bool {
	if len(prefixes) == 0 {
		return true
	}

	for _, prefix := range prefixes {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

func patternsFrom(differ Differ, prefixes []string) ([]string, error) {
	dirs, err := differ.Diff()
	if err != nil {
		return nil, err
	}
	files, err := differ.DiffFiles()
	if err != nil {
		return nil, err
	}

	patterns := make([]string, 0, len(files))
	for f := range files {
		if d := dirs[filepath.Dir(f)]; !d.Exists {
			continue
		}
		patterns = append(patterns, "file="+f)
	}

	return append(patterns, prefixes...), nil
}
