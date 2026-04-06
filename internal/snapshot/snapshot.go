// Package snapshot provides a filesystem snapshot engine for context injection.
// It scans directory trees, computes content hashes, and produces structured
// snapshots that can be scoped down to specific files or subtrees.
package snapshot

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/crush/internal/fsext"
)

// FileMeta holds metadata about a single file in the snapshot.
type FileMeta struct {
	Path    string    `json:"path"`     // relative to root
	Hash    string    `json:"hash"`     // sha256 of content
	Size    int64     `json:"size"`     // bytes
	ModTime time.Time `json:"mod_time"` // last modification
	IsDir   bool      `json:"is_dir"`
}

// TreeNode represents a node in the directory tree.
type TreeNode struct {
	Name     string      `json:"name"`
	Path     string      `json:"path"` // relative to root
	IsDir    bool        `json:"is_dir"`
	Size     int64       `json:"size"`
	Children []*TreeNode `json:"children,omitempty"`
}

// Snapshot represents a point-in-time view of the filesystem.
type Snapshot struct {
	ID        string               `json:"id"`    // hash of all file hashes + structure
	Root      string               `json:"root"`  // absolute root path
	Files     map[string]*FileMeta `json:"files"` // path -> metadata
	Tree      *TreeNode            `json:"tree"`  // directory tree
	CreatedAt time.Time            `json:"created_at"`
}

// ScopeSelection tracks which files/dirs the user has selected or ignored.
type ScopeSelection struct {
	mu       sync.RWMutex
	root     string
	selected map[string]bool // path -> true=included, false=excluded
	ignored  map[string]bool // path -> true=ignored (pattern-based)
}

// NewScopeSelection creates a new scope selection rooted at the given path.
func NewScopeSelection(root string) *ScopeSelection {
	return &ScopeSelection{
		root:     root,
		selected: make(map[string]bool),
		ignored:  make(map[string]bool),
	}
}

// Toggle toggles a path's selection state. Returns the new state.
func (s *ScopeSelection) Toggle(path string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, exists := s.selected[path]
	if exists && current {
		delete(s.selected, path)
		return false
	}
	s.selected[path] = true
	return true
}

// IsSelected returns whether a path is explicitly selected.
func (s *ScopeSelection) IsSelected(path string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.selected[path]
}

// SetIgnored marks a path as ignored.
func (s *ScopeSelection) SetIgnored(path string, ignored bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ignored {
		s.ignored[path] = true
	} else {
		delete(s.ignored, path)
	}
}

// IsIgnored returns whether a path is ignored.
func (s *ScopeSelection) IsIgnored(path string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ignored[path]
}

// SelectedPaths returns all explicitly selected paths.
func (s *ScopeSelection) SelectedPaths() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	paths := make([]string, 0, len(s.selected))
	for p, sel := range s.selected {
		if sel {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	return paths
}

// IgnoredPaths returns all explicitly ignored paths.
func (s *ScopeSelection) IgnoredPaths() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	paths := make([]string, 0, len(s.ignored))
	for p := range s.ignored {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}

// SelectAll marks all files in the snapshot as selected.
func (s *ScopeSelection) SelectAll(snap *Snapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for path, meta := range snap.Files {
		if !meta.IsDir && !s.ignored[path] {
			s.selected[path] = true
		}
	}
}

// ClearSelection removes all selections.
func (s *ScopeSelection) ClearSelection() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.selected = make(map[string]bool)
}

// SelectedCount returns the number of selected files.
func (s *ScopeSelection) SelectedCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	n := 0
	for _, sel := range s.selected {
		if sel {
			n++
		}
	}
	return n
}

// TakeSnapshot scans the filesystem at root and produces a snapshot.
// It respects gitignore and common ignore patterns via fsext.
func TakeSnapshot(root string) (*Snapshot, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve root: %w", err)
	}

	snap := &Snapshot{
		Root:      root,
		Files:     make(map[string]*FileMeta),
		CreatedAt: time.Now(),
	}

	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip errors
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		if rel == "." {
			return nil
		}

		// Use fsext's ignore logic (gitignore, crushignore, common patterns)
		if fsext.ShouldExcludeFile(root, path) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		meta := &FileMeta{
			Path:    rel,
			Size:    info.Size(),
			ModTime: info.ModTime(),
			IsDir:   d.IsDir(),
		}

		if !d.IsDir() {
			// Compute content hash lazily (only hash, don't store content)
			hash, herr := hashFile(path)
			if herr == nil {
				meta.Hash = hash
			}
		}

		snap.Files[rel] = meta
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk directory: %w", err)
	}

	// Build tree
	snap.Tree = buildTree(snap.Files, filepath.Base(root))

	// Compute snapshot ID from all file hashes
	snap.ID = computeSnapshotID(snap.Files)

	return snap, nil
}

// hashFile computes a SHA-256 hash of a file's contents.
func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:16]), nil // truncated for compactness
}

// computeSnapshotID computes a deterministic ID from all file hashes.
func computeSnapshotID(files map[string]*FileMeta) string {
	var paths []string
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	h := sha256.New()
	for _, p := range paths {
		fmt.Fprintf(h, "%s:%s\n", p, files[p].Hash)
	}
	return hex.EncodeToString(h.Sum(nil)[:8])
}

// buildTree constructs a TreeNode hierarchy from flat file metadata.
func buildTree(files map[string]*FileMeta, rootName string) *TreeNode {
	root := &TreeNode{
		Name:  rootName,
		Path:  ".",
		IsDir: true,
	}

	// Sort paths for deterministic tree
	var paths []string
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	nodeMap := map[string]*TreeNode{".": root}

	for _, path := range paths {
		meta := files[path]
		parts := strings.Split(filepath.ToSlash(path), "/")

		// Ensure parent directories exist
		for i := range len(parts) - 1 {
			dirPath := strings.Join(parts[:i+1], "/")
			if _, exists := nodeMap[dirPath]; !exists {
				dirNode := &TreeNode{
					Name:  parts[i],
					Path:  dirPath,
					IsDir: true,
				}
				parentPath := "."
				if i > 0 {
					parentPath = strings.Join(parts[:i], "/")
				}
				if parent, ok := nodeMap[parentPath]; ok {
					parent.Children = append(parent.Children, dirNode)
				}
				nodeMap[dirPath] = dirNode
			}
		}

		node := &TreeNode{
			Name:  parts[len(parts)-1],
			Path:  filepath.ToSlash(path),
			IsDir: meta.IsDir,
			Size:  meta.Size,
		}

		parentPath := "."
		if len(parts) > 1 {
			parentPath = strings.Join(parts[:len(parts)-1], "/")
		}
		if parent, ok := nodeMap[parentPath]; ok {
			if !meta.IsDir {
				parent.Children = append(parent.Children, node)
			}
		}
		if meta.IsDir {
			nodeMap[filepath.ToSlash(path)] = node
		}
	}

	// Sort children: dirs first, then by name
	sortTree(root)
	return root
}

func sortTree(node *TreeNode) {
	sort.Slice(node.Children, func(i, j int) bool {
		if node.Children[i].IsDir != node.Children[j].IsDir {
			return node.Children[i].IsDir
		}
		return node.Children[i].Name < node.Children[j].Name
	})
	for _, child := range node.Children {
		if child.IsDir {
			sortTree(child)
		}
	}
}

// FileCount returns the number of non-directory files.
func (s *Snapshot) FileCount() int {
	n := 0
	for _, m := range s.Files {
		if !m.IsDir {
			n++
		}
	}
	return n
}

// ReadFileContent reads the actual content of a file from disk.
// Content is loaded lazily, not stored in the snapshot.
func (s *Snapshot) ReadFileContent(relPath string) (string, error) {
	absPath := filepath.Join(s.Root, relPath)
	data, err := os.ReadFile(absPath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
