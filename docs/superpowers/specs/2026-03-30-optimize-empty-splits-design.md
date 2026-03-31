# Fix: optimize.New panics on empty splits slice

GitHub issue: #106

## Problem

`Optimizer.Configurations()` indexes `opt.splits[0]` without checking that the slice is non-empty. An `Optimizer` created with an empty splits slice will panic with an index-out-of-range error when `Configurations` is called.

## Fix

Add a guard at the top of `Configurations()` that returns a descriptive error when `len(opt.splits) == 0`. This matches the existing pattern -- `Configurations` already returns `([]study.RunConfig, error)`, so no signature changes are needed. No other callers are affected.

## Test

Add a test case that calls `Configurations` on an `Optimizer` created with an empty (nil) splits slice and verifies it returns an error instead of panicking.
