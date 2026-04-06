// Package delegation provides multi-agent task orchestration for complex coding tasks.
// When a user requests a complex task, the agent can propose a delegation plan
// where sub-tasks are assigned to different models/providers and executed concurrently
// on independent git branches with conflict-free merging.
package delegation

import (
	"context"
	"time"

	"github.com/charmbracelet/crush/internal/config"
)

// DelegationPlan represents a breakdown of a complex task into sub-tasks
// assigned to different models/providers.
type DelegationPlan struct {
	// ID is a unique identifier for this plan
	ID string

	// OriginalTask is the user's original request
	OriginalTask string

	// Rationale explains why and how the task was decomposed
	Rationale string

	// SubTasks are the individual tasks to be executed in parallel
	SubTasks []SubTask

	// Dependencies describe task ordering (if any)
	Dependencies map[string][]string // task_id -> list of task_ids it depends on

	// CreatedAt is when this plan was generated
	CreatedAt time.Time

	// ApprovedBy indicates user approval
	ApprovedBy bool

	// Modifications is user-provided changes to the plan
	Modifications string
}

// SubTask represents a single task assigned to a model.
type SubTask struct {
	// ID is a unique identifier within the plan (e.g., "task_1", "task_2")
	ID string

	// Title is a short description of this sub-task
	Title string

	// Description is the detailed task specification
	Description string

	// AssignedModel is the target model for this task
	AssignedModel config.SelectedModel

	// AssignedProvider is the provider ID (e.g., "openai", "anthropic")
	AssignedProvider string

	// Scope defines which files/directories this agent will modify
	Scope SubTaskScope

	// EstimatedComplexity is a rough estimate of task complexity (1-10)
	EstimatedComplexity int

	// Status tracks execution state
	Status SubTaskStatus

	// BranchName is the git branch this agent works on
	BranchName string

	// CreatedAt is when this sub-task was created
	CreatedAt time.Time

	// StartedAt is when execution began
	StartedAt *time.Time

	// CompletedAt is when execution finished
	CompletedAt *time.Time

	// Result contains the agent's output and changes
	Result *SubTaskResult
}

// SubTaskScope defines the file/module boundaries for a sub-task.
type SubTaskScope struct {
	// Paths are glob patterns for files this agent can modify
	// Example: ["src/core/**/*.go", "internal/auth/**/*.go"]
	Paths []string

	// ExcludePaths are glob patterns to explicitly exclude
	ExcludePaths []string

	// CreatedFiles are file paths the agent is expected to create
	CreatedFiles []string

	// ModifiedFiles are file paths the agent is expected to modify
	ModifiedFiles []string

	// Description explains the scope in human terms
	Description string
}

// SubTaskStatus tracks the execution state of a sub-task.
type SubTaskStatus string

const (
	SubTaskPending    SubTaskStatus = "pending"
	SubTaskRunning    SubTaskStatus = "running"
	SubTaskCompleted  SubTaskStatus = "completed"
	SubTaskFailed     SubTaskStatus = "failed"
	SubTaskMerged     SubTaskStatus = "merged"
	SubTaskCancelled  SubTaskStatus = "cancelled"
)

// SubTaskResult holds the outcome of a sub-task execution.
type SubTaskResult struct {
	// Output is the agent's response/summary
	Output string

	// CommitHash is the final commit on the sub-task branch
	CommitHash string

	// ChangedFiles lists files modified by this agent
	ChangedFiles []FileChange

	// Issues are any problems encountered during execution
	Issues []string

	// ConflictsWith lists other sub-tasks that have conflicting changes
	ConflictsWith []string

	// SummaryForMerge is a brief summary for merge review
	SummaryForMerge string
}

// FileChange describes how a file was modified.
type FileChange struct {
	Path      string // relative path
	Operation string // "created", "modified", "deleted"
	Size      int64  // file size
	Lines     int    // line count
}

// SubAgentExecution represents a running sub-agent instance.
type SubAgentExecution struct {
	// SubTask is the task being executed
	SubTask *SubTask

	// AgentID is the internal agent instance ID
	AgentID string

	// BranchName is the git branch this agent operates on
	BranchName string

	// Messages are the chat messages in this agent's conversation
	Messages []SubAgentMessage

	// Progress tracks execution progress
	Progress SubAgentProgress

	// Context is the execution context (can be cancelled)
	Context context.Context

	// Cancel cancels this sub-agent
	Cancel context.CancelFunc
}

// SubAgentMessage is a single message in a sub-agent's conversation.
type SubAgentMessage struct {
	Timestamp time.Time
	Role      string // "user", "assistant", "system"
	Content   string
}

// SubAgentProgress tracks execution progress.
type SubAgentProgress struct {
	// PercentComplete is 0-100
	PercentComplete int

	// CurrentStep is a human-readable description of what's being done
	CurrentStep string

	// TokensUsed is the total tokens consumed so far
	TokensUsed int64

	// TokenBudget is the allocated token budget for this agent
	TokenBudget int64

	// LastUpdate is when progress was last updated
	LastUpdate time.Time
}

// DelegationCoordinator manages all active sub-agents and their coordination.
type DelegationCoordinator struct {
	// MainAgentID is the ID of the primary agent
	MainAgentID string

	// Plan is the delegation plan being executed
	Plan *DelegationPlan

	// ActiveAgents maps sub-task ID to its execution
	ActiveAgents map[string]*SubAgentExecution

	// MergeStrategy defines how sub-agent changes will be merged
	MergeStrategy MergeStrategy

	// ConflictDetected indicates if any conflicts have been found
	ConflictDetected bool

	// ConflictDetails describes conflicts and proposed resolutions
	ConflictDetails []ConflictDetail
}

// MergeStrategy defines how to merge sub-agent changes safely.
type MergeStrategy struct {
	// AllowAutoMerge indicates if changes can be auto-merged
	AllowAutoMerge bool

	// RequireReview requires manual review before merging
	RequireReview bool

	// ConflictResolutionMode: "abort", "manual", "agent_decides"
	ConflictResolutionMode string

	// MergeOrder specifies the order to merge branches (empty = parallel)
	MergeOrder []string

	// PreMergeTests are tests to run before merging
	PreMergeTests []string

	// PostMergeTests are tests to run after merging
	PostMergeTests []string
}

// ConflictDetail describes a detected conflict.
type ConflictDetail struct {
	// File is the conflicted file path
	File string

	// TaskA and TaskB are the conflicting sub-tasks
	TaskA string
	TaskB string

	// LineRangeA is the line range in TaskA's version
	LineRangeA string

	// LineRangeB is the line range in TaskB's version
	LineRangeB string

	// ProposedResolution is a suggested fix
	ProposedResolution string

	// Severity: "critical", "high", "medium", "low"
	Severity string
}

// DecompositionAnalysis is the result of analyzing a task for decomposition.
type DecompositionAnalysis struct {
	// CanDecompose indicates if the task is suitable for delegation
	CanDecompose bool

	// Reason explains whether/why decomposition is recommended
	Reason string

	// ProposedPlan is the suggested delegation plan
	ProposedPlan *DelegationPlan

	// Confidence is how confident the agent is in this decomposition (0-100)
	Confidence int

	// AlternativePlans are other possible decompositions
	AlternativePlans []*DelegationPlan
}
