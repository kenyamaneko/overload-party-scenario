package apiscenario

// NewLockReasonLevel builds a LockReason for an unmet level requirement.
func NewLockReasonLevel(required, current int64) LockReason {
	return LockReason{Type: "level", Required: required, Current: current}
}

// NewLockReasonFaction builds a LockReason for a missing required faction.
func NewLockReasonFaction(faction string) LockReason {
	return LockReason{Type: "faction", Required: faction}
}

// NewLockReasonEpisode builds a LockReason for an uncompleted prerequisite episode.
func NewLockReasonEpisode(episodeID string) LockReason {
	return LockReason{Type: "episode", Required: episodeID}
}
