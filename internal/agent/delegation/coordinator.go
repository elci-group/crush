package delegation

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Coordinator manages concurrent sub-agent execution and coordination.
type Coordinator struct {
	plan              *DelegationPlan
	mainAgentID       string
	activeAgents      map[string]*SubAgentExecution
	mergeStrategy     MergeStrategy
	conflictDetected  bool
	conflictDetails   []ConflictDetail
	mu                sync.RWMutex
	ctx               context.Context
	cancel            context.CancelFunc
	branchManager     *BranchManager
	conflictResolver  *ConflictResolver
}

// NewCoordinator creates a new delegation coordinator.
func NewCoordinator(plan *DelegationPlan, mainAgentID string) *Coordinator {
	ctx, cancel := context.WithCancel(context.Background())
	return &Coordinator{
		plan:            plan,
		mainAgentID:     mainAgentID,
		activeAgents:    make(map[string]*SubAgentExecution),
		mergeStrategy:   defaultMergeStrategy(),
		ctx:             ctx,
		cancel:          cancel,
		branchManager:   NewBranchManager(),
		conflictResolver: NewConflictResolver(),
	}
}

// defaultMergeStrategy returns a safe-by-default merge strategy.
func defaultMergeStrategy() MergeStrategy {
	return MergeStrategy{
		AllowAutoMerge:         false,
		RequireReview:          true,
		ConflictResolutionMode: "manual",
		MergeOrder:             []string{}, // parallel by default
	}
}

// StartAgent initializes and runs a sub-agent for the given task.
func (c *Coordinator) StartAgent(task *SubTask) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.activeAgents[task.ID]; exists {
		return fmt.Errorf("agent for task %s already running", task.ID)
	}

	// Create branch for this agent
	branchName := task.BranchName
	if err := c.branchManager.CreateBranch(branchName); err != nil {
		return fmt.Errorf("failed to create branch %s: %w", branchName, err)
	}

	// Create execution context
	agentCtx, cancel := context.WithCancel(c.ctx)
	execution := &SubAgentExecution{
		SubTask:     task,
		AgentID:     fmt.Sprintf("agent_%s", task.ID),
		BranchName:  branchName,
		Messages:    []SubAgentMessage{},
		Progress:    SubAgentProgress{PercentComplete: 0},
		Context:     agentCtx,
		Cancel:      cancel,
	}

	c.activeAgents[task.ID] = execution

	// Start execution in goroutine
	go c.runAgent(execution)

	return nil
}

// runAgent executes a single sub-agent task.
func (c *Coordinator) runAgent(exec *SubAgentExecution) {
	defer func() {
		c.mu.Lock()
		exec.SubTask.Status = SubTaskCompleted
		exec.SubTask.CompletedAt = timePtr(time.Now())
		c.mu.Unlock()
	}()

	exec.SubTask.Status = SubTaskRunning
	exec.SubTask.StartedAt = timePtr(time.Now())

	// Switch to task branch
	if err := c.branchManager.CheckoutBranch(exec.BranchName); err != nil {
		exec.SubTask.Status = SubTaskFailed
		if exec.SubTask.Result == nil {
			exec.SubTask.Result = &SubTaskResult{}
		}
		exec.SubTask.Result.Issues = append(exec.SubTask.Result.Issues, fmt.Sprintf("Failed to checkout branch: %v", err))
		return
	}

	// Simulate agent work (in real implementation, would spawn actual agent process)
	c.simulateAgentWork(exec)

	// Detect conflicts with other agents
	c.detectConflicts(exec)

	// Commit changes to branch
	if err := c.branchManager.CommitChanges(exec.BranchName, fmt.Sprintf("Complete %s", exec.SubTask.Title)); err != nil {
		exec.SubTask.Status = SubTaskFailed
		if exec.SubTask.Result == nil {
			exec.SubTask.Result = &SubTaskResult{}
		}
		exec.SubTask.Result.Issues = append(exec.SubTask.Result.Issues, fmt.Sprintf("Failed to commit: %v", err))
	}
}

// simulateAgentWork simulates task execution (placeholder for actual agent integration).
func (c *Coordinator) simulateAgentWork(exec *SubAgentExecution) {
	task := exec.SubTask
	task.Result = &SubTaskResult{
		Output:           fmt.Sprintf("Completed %s on branch %s", task.Title, exec.BranchName),
		ChangedFiles:     []FileChange{},
		Issues:           []string{},
		SummaryForMerge:  fmt.Sprintf("Implemented %s module with scope: %s", task.ID, task.Scope.Description),
	}

	// Update progress
	for i := 0; i <= 100; i += 10 {
		select {
		case <-exec.Context.Done():
			return
		default:
			exec.Progress.PercentComplete = i
			exec.Progress.CurrentStep = fmt.Sprintf("Processing %s...", task.Title)
			exec.Progress.LastUpdate = time.Now()
			time.Sleep(100 * time.Millisecond)
		}
	}
	exec.Progress.PercentComplete = 100
}

// detectConflicts checks for file conflicts between completed agents.
func (c *Coordinator) detectConflicts(exec *SubAgentExecution) {
	c.mu.Lock()
	defer c.mu.Unlock()

	conflicts := c.conflictResolver.DetectConflicts(exec.SubTask, c.plan.SubTasks)
	if len(conflicts) > 0 {
		c.conflictDetected = true
		c.conflictDetails = append(c.conflictDetails, conflicts...)
		exec.SubTask.Result.ConflictsWith = conflictTaskIDs(conflicts)
	}
}

// conflictTaskIDs extracts task IDs from conflict details.
func conflictTaskIDs(details []ConflictDetail) []string {
	seen := make(map[string]bool)
	var ids []string
	for _, d := range details {
		if !seen[d.TaskB] && d.TaskA != d.TaskB {
			ids = append(ids, d.TaskB)
			seen[d.TaskB] = true
		}
	}
	return ids
}

// WaitAll blocks until all agents complete or context is cancelled.
func (c *Coordinator) WaitAll() error {
	for {
		c.mu.RLock()
		allDone := true
		for _, exec := range c.activeAgents {
			if exec.SubTask.Status != SubTaskCompleted && exec.SubTask.Status != SubTaskFailed {
				allDone = false
				break
			}
		}
		c.mu.RUnlock()

		if allDone {
			break
		}

		select {
		case <-c.ctx.Done():
			return c.ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}

	return nil
}

// MergeResults attempts to merge all sub-agent branches back to main.
func (c *Coordinator) MergeResults() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conflictDetected && c.mergeStrategy.ConflictResolutionMode == "abort" {
		return fmt.Errorf("conflicts detected and conflict resolution mode is abort")
	}

	// Build merge order (dependencies first)
	mergeOrder := c.buildMergeOrder()

	for _, taskID := range mergeOrder {
		exec, exists := c.activeAgents[taskID]
		if !exists {
			continue
		}

		if exec.SubTask.Status == SubTaskFailed {
			return fmt.Errorf("cannot merge: task %s failed", taskID)
		}

		// Merge branch to main
		if err := c.branchManager.MergeBranch(exec.BranchName); err != nil {
			if c.mergeStrategy.ConflictResolutionMode == "manual" {
				return fmt.Errorf("merge conflict in %s: %w (manual resolution required)", exec.BranchName, err)
			}
			// agent_decides mode: attempt auto-resolution
			if err := c.conflictResolver.ResolveConflict(exec.SubTask, c.plan.SubTasks); err != nil {
				return fmt.Errorf("failed to auto-resolve conflict in %s: %w", exec.BranchName, err)
			}
			if err := c.branchManager.MergeBranch(exec.BranchName); err != nil {
				return fmt.Errorf("merge still failed after resolution: %w", err)
			}
		}

		exec.SubTask.Status = SubTaskMerged
	}

	return nil
}

// buildMergeOrder determines safe merge order based on dependencies.
func (c *Coordinator) buildMergeOrder() []string {
	if len(c.mergeStrategy.MergeOrder) > 0 {
		return c.mergeStrategy.MergeOrder
	}

	// Topological sort of tasks by dependencies
	var order []string
	merged := make(map[string]bool)

	for len(order) < len(c.plan.SubTasks) {
		for _, task := range c.plan.SubTasks {
			if merged[task.ID] {
				continue
			}

			deps := c.plan.Dependencies[task.ID]
			allDepsMerged := true
			for _, dep := range deps {
				if !merged[dep] {
					allDepsMerged = false
					break
				}
			}

			if allDepsMerged {
				order = append(order, task.ID)
				merged[task.ID] = true
			}
		}
	}

	return order
}

// GetStatus returns current execution status.
func (c *Coordinator) GetStatus() DelegationStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()

	status := DelegationStatus{
		Plan:              c.plan,
		ActiveCount:       0,
		CompletedCount:    0,
		FailedCount:       0,
		ConflictDetected:  c.conflictDetected,
		ConflictDetails:   c.conflictDetails,
		LastUpdate:        time.Now(),
	}

	for _, exec := range c.activeAgents {
		switch exec.SubTask.Status {
		case SubTaskRunning:
			status.ActiveCount++
		case SubTaskCompleted, SubTaskMerged:
			status.CompletedCount++
		case SubTaskFailed:
			status.FailedCount++
		}
	}

	return status
}

// Cancel stops all running agents.
func (c *Coordinator) Cancel() {
	c.cancel()
	c.mu.Lock()
	for _, exec := range c.activeAgents {
		if exec.Cancel != nil {
			exec.Cancel()
		}
	}
	c.mu.Unlock()
}

// DelegationStatus captures current execution state.
type DelegationStatus struct {
	Plan             *DelegationPlan
	ActiveCount      int
	CompletedCount   int
	FailedCount      int
	ConflictDetected bool
	ConflictDetails  []ConflictDetail
	LastUpdate       time.Time
}

// BranchManager handles git branch operations for sub-agents.
type BranchManager struct {
	mu       sync.Mutex
	branches map[string]bool
}

// NewBranchManager creates a new branch manager.
func NewBranchManager() *BranchManager {
	return &BranchManager{
		branches: make(map[string]bool),
	}
}

// CreateBranch creates a new branch for a sub-agent.
func (bm *BranchManager) CreateBranch(name string) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if bm.branches[name] {
		return fmt.Errorf("branch %s already exists", name)
	}

	// git checkout -b <branch>
	cmd := exec.Command("git", "checkout", "-b", name)
	if err := cmd.Run(); err != nil {
		return err
	}

	bm.branches[name] = true
	return nil
}

// CheckoutBranch switches to an existing branch.
func (bm *BranchManager) CheckoutBranch(name string) error {
	cmd := exec.Command("git", "checkout", name)
	return cmd.Run()
}

// CommitChanges commits staged changes to a branch.
func (bm *BranchManager) CommitChanges(branch, message string) error {
	// git add .
	if err := exec.Command("git", "add", ".").Run(); err != nil {
		return err
	}

	// git commit
	cmd := exec.Command("git", "commit", "-m", message)
	return cmd.Run()
}

// MergeBranch merges a branch back to main.
func (bm *BranchManager) MergeBranch(name string) error {
	// Switch back to main
	if err := exec.Command("git", "checkout", "main").Run(); err != nil {
		// Try master if main doesn't exist
		exec.Command("git", "checkout", "master").Run()
	}

	// git merge <branch>
	cmd := exec.Command("git", "merge", name)
	return cmd.Run()
}

// ConflictResolver handles merge conflict detection and resolution.
type ConflictResolver struct {
	mu sync.Mutex
}

// NewConflictResolver creates a new conflict resolver.
func NewConflictResolver() *ConflictResolver {
	return &ConflictResolver{}
}

// DetectConflicts identifies file conflicts between a completed task and others.
func (cr *ConflictResolver) DetectConflicts(completed *SubTask, allTasks []SubTask) []ConflictDetail {
	cr.mu.Lock()
	defer cr.mu.Unlock()

	var conflicts []ConflictDetail

	completedPaths := make(map[string]bool)
	for _, path := range completed.Scope.Paths {
		completedPaths[path] = true
	}

	for _, other := range allTasks {
		if other.ID == completed.ID || other.Status != SubTaskCompleted {
			continue
		}

		// Check for overlapping scopes
		for _, otherPath := range other.Scope.Paths {
			if completedPaths[otherPath] {
				conflicts = append(conflicts, ConflictDetail{
					File:               otherPath,
					TaskA:              completed.ID,
					TaskB:              other.ID,
					Severity:           "high",
					ProposedResolution: fmt.Sprintf("Manual review required for %s between %s and %s", otherPath, completed.ID, other.ID),
				})
			}
		}
	}

	return conflicts
}

// ResolveConflict attempts automatic conflict resolution.
func (cr *ConflictResolver) ResolveConflict(task *SubTask, allTasks []SubTask) error {
	cr.mu.Lock()
	defer cr.mu.Unlock()

	// Check git status for conflicts
	cmd := exec.Command("git", "diff", "--name-only", "--diff-filter=U")
	output, err := cmd.Output()
	if err != nil {
		return err
	}

	conflictFiles := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(conflictFiles) == 0 || conflictFiles[0] == "" {
		return nil // no conflicts
	}

	// For now, use "theirs" strategy (accept current branch)
	for _, file := range conflictFiles {
		exec.Command("git", "checkout", "--theirs", file).Run()
	}

	// Stage resolved files
	if err := exec.Command("git", "add", ".").Run(); err != nil {
		return err
	}

	return nil
}

// Helper function to create time pointers.
func timePtr(t time.Time) *time.Time {
	return &t
}
