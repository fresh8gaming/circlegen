/*
Copyright 2016 The gta AUTHORS. All rights reserved.

Use of this source code is governed by the Apache 2 license that can be found
in the LICENSE file.
*/
package gta

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// A Differ implements provides methods that return values to understand the
// directories and files that have changed.
// and the dirs they in which occur.
type Differ interface {
	// Diff returns a set of absolute pathed directories that have files that
	// have been modified.
	Diff() (map[string]Directory, error)

	// DiffFiles returns a map whose keys are absolute files paths. A map value
	// is true when the file exists.
	DiffFiles() (map[string]bool, error)
}

// GitDifferOption is an option function used to modify a git differ
type GitDifferOption func(*git)

// SetUseMergeCommit sets the useMergeCommit field on a git differ
func SetUseMergeCommit(useMergeCommit bool) GitDifferOption {
	return func(gd *git) {
		gd.useMergeCommit = useMergeCommit
	}
}

// SetBaseBranch sets the baseBranch field on a git differ
func SetBaseBranch(baseBranch string) GitDifferOption {
	return func(gd *git) {
		gd.baseBranch = baseBranch
	}
}

// NewGitDiffer returns a Differ that determines differences using git.
func NewGitDiffer(opts ...GitDifferOption) Differ {
	g := &git{
		useMergeCommit: false,
		baseBranch:     "origin/master",
	}

	for _, opt := range opts {
		opt(g)
	}

	return &differ{
		diff: g.diff,
	}
}

// NewFileDiffer returns a Differ that operates on a list of absolute paths of
// changed files.
func NewFileDiffer(files []string) Differ {
	m := make(map[string]struct{}, len(files))

	for _, v := range files {
		m[v] = struct{}{}
	}

	return &differ{
		diff: func() (map[string]struct{}, error) { return m, nil },
	}
}

type differ struct {
	diff func() (map[string]struct{}, error)
}

// git implements the Differ interface using a git version control method.
type git struct {
	baseBranch     string
	useMergeCommit bool
	onceDiff       sync.Once
	changedFiles   map[string]struct{}
	diffErr        error
}

// A Directory describes changes to a directory and its contents.
type Directory struct {
	Exists bool
	Files  []string
}

// Diff returns a set of changed directories. The keys of the returned map are
// absolute paths.
func (d *differ) Diff() (map[string]Directory, error) {
	files, err := d.diff()
	if err != nil {
		return nil, err
	}

	existsDirs := make(map[string]Directory, len(files))
	for abs := range files {
		absdir := filepath.Dir(abs)
		dir, ok := existsDirs[absdir]
		if !ok {
			dir.Exists = exists(absdir)
		}

		fn := filepath.Base(abs)
		dir.Files = append(dir.Files, fn)
		existsDirs[absdir] = dir
	}

	return existsDirs, nil
}

// DiffFiles returns a set of changed files. The keys of the returned map are
// absolute paths. The map values indicate whether or not the file exists: a
// false value means the file was deleted.
func (d *differ) DiffFiles() (map[string]bool, error) {
	files, err := d.diff()
	if err != nil {
		return nil, err
	}

	existsFiles := map[string]bool{}
	for abs := range files {
		existsFiles[abs] = exists(abs)
	}

	return existsFiles, nil
}

func getMergeParents() (parent1 string, rightwardParents []string, err error) {
	out, err := exec.Command("git", "log", "-1", "--pretty=format:%p").Output()
	if err != nil {
		return
	}
	parents := strings.TrimSpace(string(out))
	parentSplit := strings.Split(parents, " ")

	// for merge commits, parents will include both values
	if len(parentSplit) >= 2 {
		parent1 = parentSplit[0]
		rightwardParents = parentSplit[1:]
		return
	}

	// for squash-merge/rebase commits, get the most recent merge commit hash and use as left parent
	out, err = exec.Command("git", "log", "-1", "--merges", "--pretty=format:%h").Output()
	if err != nil {
		return
	}
	parent1 = strings.TrimSpace(string(out))
	rightwardParents = []string{"HEAD"}
	return
}

// diff returns a set of changed files.
func (g *git) diff() (map[string]struct{}, error) {
	g.onceDiff.Do(func() {
		files, err := func() (map[string]struct{}, error) {
			// We get the root of the repository to build our full path.
			out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
			if err != nil {
				return nil, err
			}
			root := strings.TrimSpace(string(out))
			parent1 := g.baseBranch
			rightwardParents := []string{"HEAD"}
			if g.useMergeCommit {
				parent1, rightwardParents, err = getMergeParents()
				if err != nil {
					return nil, err
				}
			}

			files := make(map[string]struct{})

			for _, parent2 := range rightwardParents {
				// get the names of all affected files without doing rename detection.
				cmd := exec.Command("git", "diff", fmt.Sprintf("%s...%s", parent1, parent2), "--name-only", "--no-renames")
				stdout, err := cmd.StdoutPipe()
				if err != nil {
					return nil, err
				}

				if err := cmd.Start(); err != nil {
					return nil, err
				}

				changedPaths, err := diffPaths(root, stdout)
				if err != nil {
					return nil, err
				}

				for path := range changedPaths {
					files[path] = struct{}{}
				}

				err = cmd.Wait()
				if err != nil {
					return nil, err
				}
			}
			return files, nil
		}()
		if err != nil {
			g.diffErr = err
			return
		}

		g.changedFiles = files
	})

	return g.changedFiles, g.diffErr
}

// diffPaths returns the path that have changed.
func diffPaths(root string, r io.Reader) (map[string]struct{}, error) {
	paths := make(map[string]struct{})

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		path := scanner.Text()

		// We build our full absolute file path.
		full, err := filepath.Abs(filepath.Join(root, path))
		if err != nil {
			return nil, err
		}

		paths[full] = struct{}{}
	}

	return paths, scanner.Err()
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

type fileDiffer struct {
	changedFiles map[string]struct{}
}
