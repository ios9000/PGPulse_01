package remediation

// Urgency score constants for priority levels.
const (
	UrgencyInfo           = 1.0
	UrgencySuggestion     = 3.0
	UrgencyActionRequired = 5.0
	UrgencySoftCap        = 10.0
)

// UrgencyFromPriority returns the base urgency score for a priority level.
func UrgencyFromPriority(p Priority) float64 {
	switch p {
	case PriorityActionRequired:
		return UrgencyActionRequired
	case PrioritySuggestion:
		return UrgencySuggestion
	case PriorityInfo:
		return UrgencyInfo
	default:
		return UrgencySuggestion
	}
}
