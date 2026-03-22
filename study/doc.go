// Package study provides a framework for running a strategy multiple times
// with different configurations and synthesizing the results into a report.
// Parameter sweeps are cross-producted with study configurations to produce
// the run matrix. Results are collected and passed to a study-specific
// Analyze function that composes a report from report primitives.
package study
