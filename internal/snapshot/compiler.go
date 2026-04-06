package snapshot

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
)

// activeContext holds the current injection context string, set by the TUI
// and read by the prompt builder. Using atomic.Value for thread safety.
var activeContext atomic.Value // string

// SetActiveContext sets the current injection context for prompt building.
func SetActiveContext(ctx string) {
	activeContext.Store(ctx)
}

// ActiveContext returns the current injection context string.
func ActiveContext() string {
	v := activeContext.Load()
	if v == nil {
		return ""
	}
	return v.(string)
}

// ClearActiveContext removes the current injection context.
func ClearActiveContext() {
	activeContext.Store("")
}

// TokenBudget defines limits for context compilation.
type TokenBudget struct {
	MaxTokens    int // hard cap on estimated tokens
	MaxFiles     int // max number of full-content files (0 = unlimited)
	MaxFileBytes int // max bytes per file (0 = unlimited)
}

// DefaultBudget returns a sensible default token budget.
func DefaultBudget() TokenBudget {
	return TokenBudget{
		MaxTokens:    20000,
		MaxFiles:     50,
		MaxFileBytes: 64 * 1024, // 64KB per file
	}
}

// CompiledContext is the structured output ready for prompt injection.
type CompiledContext struct {
	SnapshotID    string
	TreeSummary   string        // level 1: directory structure
	FileSummaries string        // level 2: file list with sizes
	FileContents  []FileContent // level 3: full content of selected files
	EstTokens     int
}

// FileContent holds a file's path and content for injection.
type FileContent struct {
	Path    string
	Content string
	Size    int64
}

// Compile takes a snapshot, scope selection, and budget, and produces
// structured context at multiple resolution levels.
func Compile(snap *Snapshot, scope *ScopeSelection, budget TokenBudget) (*CompiledContext, error) {
	if snap == nil {
		return nil, fmt.Errorf("nil snapshot")
	}

	ctx := &CompiledContext{
		SnapshotID: snap.ID,
	}

	// Level 1: Tree summary (always included, low cost)
	ctx.TreeSummary = renderTree(snap.Tree, "", true)
	ctx.EstTokens += estimateTokens(ctx.TreeSummary)

	// Level 2: File summaries for selected paths
	selected := scope.SelectedPaths()
	if len(selected) == 0 {
		return ctx, nil
	}

	var summaryLines []string
	for _, path := range selected {
		meta, ok := snap.Files[path]
		if !ok || meta.IsDir {
			continue
		}
		summaryLines = append(summaryLines, fmt.Sprintf("  %s (%s)", path, formatSize(meta.Size)))
	}
	ctx.FileSummaries = strings.Join(summaryLines, "\n")
	ctx.EstTokens += estimateTokens(ctx.FileSummaries)

	// Level 3: Full content for selected files (within budget)
	fileCount := 0
	for _, path := range selected {
		meta, ok := snap.Files[path]
		if !ok || meta.IsDir {
			continue
		}

		if budget.MaxFiles > 0 && fileCount >= budget.MaxFiles {
			break
		}

		if budget.MaxFileBytes > 0 && meta.Size > int64(budget.MaxFileBytes) {
			continue // skip oversized files
		}

		content, err := snap.ReadFileContent(path)
		if err != nil {
			continue
		}

		contentTokens := estimateTokens(content)
		if budget.MaxTokens > 0 && ctx.EstTokens+contentTokens > budget.MaxTokens {
			break // budget exhausted
		}

		ctx.FileContents = append(ctx.FileContents, FileContent{
			Path:    path,
			Content: content,
			Size:    meta.Size,
		})
		ctx.EstTokens += contentTokens
		fileCount++
	}

	return ctx, nil
}

// FormatForInjection renders the compiled context as a structured text block
// suitable for prompt injection.
func FormatForInjection(ctx *CompiledContext) string {
	var sb strings.Builder

	sb.WriteString("=== PROJECT SNAPSHOT ===\n")
	sb.WriteString(fmt.Sprintf("Snapshot ID: %s\n\n", ctx.SnapshotID))

	sb.WriteString("=== DIRECTORY STRUCTURE ===\n")
	sb.WriteString(ctx.TreeSummary)
	sb.WriteString("\n\n")

	if len(ctx.FileContents) > 0 {
		sb.WriteString(fmt.Sprintf("=== SELECTED FILES (%d) ===\n", len(ctx.FileContents)))
		for _, fc := range ctx.FileContents {
			sb.WriteString(fmt.Sprintf("\n--- %s ---\n", fc.Path))
			sb.WriteString(fc.Content)
			if !strings.HasSuffix(fc.Content, "\n") {
				sb.WriteString("\n")
			}
		}
	}

	return sb.String()
}

// renderTree renders a tree node into an indented text representation.
func renderTree(node *TreeNode, indent string, isRoot bool) string {
	if node == nil {
		return ""
	}

	var sb strings.Builder
	if isRoot {
		sb.WriteString(node.Name + "/\n")
	}

	for i, child := range node.Children {
		isLast := i == len(node.Children)-1
		connector := "├── "
		childIndent := indent + "│   "
		if isLast {
			connector = "└── "
			childIndent = indent + "    "
		}

		if child.IsDir {
			sb.WriteString(fmt.Sprintf("%s%s%s/\n", indent, connector, child.Name))
			sb.WriteString(renderTree(child, childIndent, false))
		} else {
			sb.WriteString(fmt.Sprintf("%s%s%s\n", indent, connector, child.Name))
		}
	}
	return sb.String()
}

// EstimateTokens gives a rough token count (1 token ~= 4 characters).
// This is a conservative estimate: ~1 token per 4 bytes.
func EstimateTokens(s string) int {
	return len(s) / 4
}

// estimateTokens is the internal version (used by compiler).
func estimateTokens(s string) int {
	return EstimateTokens(s)
}

// formatSize formats bytes into a human-readable string.
func formatSize(bytes int64) string {
	switch {
	case bytes >= 1024*1024:
		return fmt.Sprintf("%.1fMB", float64(bytes)/(1024*1024))
	case bytes >= 1024:
		return fmt.Sprintf("%.1fKB", float64(bytes)/1024)
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

// FlattenTree produces a flat list of tree nodes in depth-first order.
// Each entry includes its depth for rendering indentation.
type FlatNode struct {
	Node     *TreeNode
	Depth    int
	IsLast   bool
	Expanded bool
}

// FlattenTree walks the tree in DFS order producing a flat list.
func FlattenTree(root *TreeNode, expanded map[string]bool) []FlatNode {
	if root == nil {
		return nil
	}
	var result []FlatNode
	flattenChildren(root, 0, expanded, &result)
	return result
}

func flattenChildren(node *TreeNode, depth int, expanded map[string]bool, result *[]FlatNode) {
	for i, child := range node.Children {
		isLast := i == len(node.Children)-1
		isExpanded := expanded[child.Path]
		*result = append(*result, FlatNode{
			Node:     child,
			Depth:    depth,
			IsLast:   isLast,
			Expanded: isExpanded,
		})
		if child.IsDir && isExpanded {
			flattenChildren(child, depth+1, expanded, result)
		}
	}
}

// RenderTreeIndent returns the tree-drawing prefix characters for a node.
func RenderTreeIndent(depth int) string {
	return strings.Repeat("  ", depth)
}

// FileExt returns the file extension without the dot.
func FileExt(path string) string {
	ext := filepath.Ext(path)
	if ext != "" {
		return ext[1:]
	}
	return ""
}
