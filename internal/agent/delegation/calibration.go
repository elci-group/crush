package delegation

import (
	"sync"
	"time"
)

// ExecutionMetrics tracks actual outcomes of delegated tasks.
type ExecutionMetrics struct {
	// Task metadata
	TaskID        string
	OriginalScore *DelegabilityScore
	SubmittedAt   time.Time

	// Execution outcomes
	MergeConflicts    int           // Number of actual merge conflicts
	ExecutionTime     time.Duration // Time taken to complete
	TokensCost        int64         // Tokens actually consumed
	HumanIntervention bool          // Whether human had to intervene
	ActualSubtasks    int           // How many subtasks were actually created
	SuccessRate       float64       // 0.0-1.0 of sub-tasks that completed successfully

	// Computed metrics
	DelegationEffectiveness float64 // Actual benefit vs. cost
	CompletedAt             time.Time
}

// Calibrator learns from execution outcomes and adjusts scoring weights.
type Calibrator struct {
	mu               sync.RWMutex
	executionHistory []ExecutionMetrics

	// Learned weights - start with defaults, adjust over time
	weightModularity         float64
	weightIndependence       float64
	weightSurfaceArea        float64
	weightParallelEfficiency float64
	weightCognitiveLoad      float64
	weightCouplingRisk       float64

	// Threshold adjustments
	delegationThreshold float64 // Minimum score to recommend delegation
}

// NewCalibrator creates a new self-calibrating delegatability system.
func NewCalibrator() *Calibrator {
	return &Calibrator{
		executionHistory:        []ExecutionMetrics{},
		weightModularity:        0.25,
		weightIndependence:      0.2,
		weightSurfaceArea:       0.15,
		weightParallelEfficiency: 0.2,
		weightCognitiveLoad:     0.2,
		weightCouplingRisk:      0.3,
		delegationThreshold:     0.6,
	}
}

// RecordExecution logs actual execution outcomes for learning.
func (c *Calibrator) RecordExecution(metrics ExecutionMetrics) {
	c.mu.Lock()
	defer c.mu.Unlock()

	metrics.CompletedAt = time.Now()

	// Compute effectiveness
	metrics.DelegationEffectiveness = c.computeEffectiveness(metrics)

	c.executionHistory = append(c.executionHistory, metrics)

	// Calibrate weights based on this outcome
	c.recalibrate()
}

// computeEffectiveness determines if delegation was worthwhile.
// Returns 0.0-1.0, where 1.0 means perfect outcome.
func (c *Calibrator) computeEffectiveness(metrics ExecutionMetrics) float64 {
	effectiveness := 1.0

	// Penalize each merge conflict (-0.2 per conflict)
	effectiveness -= float64(metrics.MergeConflicts) * 0.2

	// Penalize human intervention (-0.3)
	if metrics.HumanIntervention {
		effectiveness -= 0.3
	}

	// Reward successful execution (+0.2 if all subtasks successful)
	if metrics.SuccessRate > 0.95 {
		effectiveness += 0.2
	}

	// Check if delegation was faster than single-agent would have been
	// Rough heuristic: single agent would take ~1.3x the time with fewer parallelization gains
	// If actual time is < 1.3x baseline, consider it effective
	estimatedSingleAgentTime := time.Duration(float64(metrics.ExecutionTime.Nanoseconds()) * 1.3)
	if metrics.ExecutionTime < estimatedSingleAgentTime {
		effectiveness += 0.15
	}

	return max(0, min(1, effectiveness))
}

// recalibrate adjusts weights based on execution history.
// This is where the system learns over time.
func (c *Calibrator) recalibrate() {
	if len(c.executionHistory) < 5 {
		return // Need minimum sample size
	}

	// Separate successful and unsuccessful delegations
	var successful, unsuccessful []ExecutionMetrics
	for _, m := range c.executionHistory {
		if m.DelegationEffectiveness > 0.7 {
			successful = append(successful, m)
		} else {
			unsuccessful = append(unsuccessful, m)
		}
	}

	// If most delegations fail, increase threshold
	if len(unsuccessful) > len(successful) {
		c.delegationThreshold = min(0.9, c.delegationThreshold+0.05)
	}

	// If most delegations succeed, lower threshold
	if len(successful) > len(unsuccessful)*2 {
		c.delegationThreshold = max(0.4, c.delegationThreshold-0.05)
	}

	// Analyze correlation between factors and success
	// (Simplified - full implementation would use statistical analysis)
	c.analyzeFactorCorrelation(successful, unsuccessful)
}

// analyzeFactorCorrelation examines which factors correlated with success.
func (c *Calibrator) analyzeFactorCorrelation(successful, unsuccessful []ExecutionMetrics) {
	if len(successful) == 0 || len(unsuccessful) == 0 {
		return
	}

	// Average modularity in successful vs unsuccessful
	avgModularitySuccess := c.averageFactor(successful, func(m ExecutionMetrics) float64 {
		return m.OriginalScore.Modularity
	})
	avgModularityUnsuccessful := c.averageFactor(unsuccessful, func(m ExecutionMetrics) float64 {
		return m.OriginalScore.Modularity
	})

	// If high modularity correlates with success, increase its weight
	if avgModularitySuccess > avgModularityUnsuccessful {
		delta := avgModularitySuccess - avgModularityUnsuccessful
		c.weightModularity = min(0.4, c.weightModularity+delta*0.05)
	}

	// Coupling risk is always bad - increase its negative weight if failures had high coupling
	avgCouplingFail := c.averageFactor(unsuccessful, func(m ExecutionMetrics) float64 {
		return m.OriginalScore.CouplingRisk
	})
	if avgCouplingFail > 0.7 {
		c.weightCouplingRisk = min(0.5, c.weightCouplingRisk+0.05)
	}
}

// averageFactor computes average of a factor across metrics.
func (c *Calibrator) averageFactor(metrics []ExecutionMetrics, fn func(ExecutionMetrics) float64) float64 {
	if len(metrics) == 0 {
		return 0
	}
	sum := 0.0
	for _, m := range metrics {
		sum += fn(m)
	}
	return sum / float64(len(metrics))
}

// GetCurrentWeights returns the current learned weights.
func (c *Calibrator) GetCurrentWeights() map[string]float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return map[string]float64{
		"modularity":          c.weightModularity,
		"independence":        c.weightIndependence,
		"surface_area":        c.weightSurfaceArea,
		"parallel_efficiency": c.weightParallelEfficiency,
		"cognitive_load":      c.weightCognitiveLoad,
		"coupling_risk":       c.weightCouplingRisk,
		"delegation_threshold": c.delegationThreshold,
	}
}

// GetDelegationThreshold returns the current delegation decision threshold.
func (c *Calibrator) GetDelegationThreshold() float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.delegationThreshold
}

// GetExecutionHistory returns all recorded executions.
func (c *Calibrator) GetExecutionHistory() []ExecutionMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.executionHistory
}

// Helper functions
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
