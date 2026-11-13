package model

const (
	StatusAvailable          = "Available"
	StatusPartiallyAvailable = "Partially available"
	StatusProcessing         = "Processing"
	StatusMissing            = "Missing"
	StatusAwaitingRelease    = "Awaiting release"
	StatusDownloading        = "Downloading"
	StatusAwaitingImport     = "Awaiting import"
	StatusQueued             = "Queued"
	StatusNeedsAttention     = "Needs attention"
	StatusPending            = "Pending"
	StatusUnknown            = "Unknown"
	StatusDeleted            = "Deleted"
)

const MissingAttentionReason = "Missing for at least 10 minutes. No active download or import was confirmed; this does not prove a search or download failed."
