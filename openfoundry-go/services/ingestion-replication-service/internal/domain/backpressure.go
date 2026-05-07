package domain

import "strings"

// DeriveBackpressureSnapshot mirrors
// event_streaming::domain::backpressure::derive_backpressure_snapshot.
//
// Pure function: turns a policy + observed backlog counters into the
// snapshot the runtime preview / catalogue surfaces.
func DeriveBackpressureSnapshot(
	policy BackpressurePolicy,
	totalBacklog int32,
	busiestStreamBacklog int32,
	activeStreams int,
) BackpressureSnapshot {
	queueCapacity := policy.QueueCapacity
	if queueCapacity < 1 {
		queueCapacity = 1
	}
	totalNonNeg := totalBacklog
	if totalNonNeg < 0 {
		totalNonNeg = 0
	}
	queueDepth := totalNonNeg
	if queueDepth > queueCapacity {
		queueDepth = queueCapacity
	}

	busiestNonNeg := busiestStreamBacklog
	if busiestNonNeg < 0 {
		busiestNonNeg = 0
	}
	maxInFlightSafe := policy.MaxInFlight
	if maxInFlightSafe < 1 {
		maxInFlightSafe = 1
	}

	queueRatio := float32(totalNonNeg) / float32(queueCapacity)
	inFlightRatio := float32(totalNonNeg) / float32(maxInFlightSafe)
	hotStreamRatio := float32(busiestNonNeg) / float32(maxInFlightSafe)
	ratio := maxF32(queueRatio, maxF32(inFlightRatio*0.65, hotStreamRatio*0.45))

	var status string
	switch {
	case ratio >= 0.8:
		status = "throttling"
	case ratio >= 0.5:
		status = "elevated"
	default:
		status = "healthy"
	}

	var strategyAdjustment float32
	if strings.EqualFold(policy.ThrottleStrategy, "drop-tail") {
		strategyAdjustment = 0.05
	}
	var throttleFactor float32
	switch status {
	case "throttling":
		throttleFactor = maxF32(0.62-strategyAdjustment, 0.4)
	case "elevated":
		throttleFactor = maxF32(0.86-strategyAdjustment, 0.65)
	default:
		throttleFactor = 1.0
	}

	var lagMS int32
	if totalBacklog > 0 {
		excess := totalBacklog - policy.MaxInFlight
		if excess < 0 {
			excess = 0
		}
		busiestForLag := busiestStreamBacklog
		if busiestForLag < 1 {
			busiestForLag = 1
		}
		streamsForLag := activeStreams
		if streamsForLag < 1 {
			streamsForLag = 1
		}
		lagMS = excess*22 + busiestForLag*9 + int32(streamsForLag)*14
	}

	return BackpressureSnapshot{
		QueueDepth:     queueDepth,
		QueueCapacity:  queueCapacity,
		LagMS:          lagMS,
		ThrottleFactor: throttleFactor,
		Status:         status,
	}
}

func maxF32(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}
