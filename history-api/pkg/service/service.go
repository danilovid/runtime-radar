package service

const (
	defaultPageSize  = 10
	defaultSliceSize = 10
	defaultOrder     = "created_at desc"
	directionLeft    = "left"
	directionRight   = "right"

	// Validation reasons shared by the runtime and the admission history services.
	reasonMissingCursor = "missing cursor"
	reasonEmptyFilter   = "at least one filter should be set"
	// EventNotFound is specially added for stats.GetLatestSeverestEvent so that client can easily determine that there's no events in the system at all.
	// Generally, API methods should return codes.NotFound without reason specified in order to indicate non-existence of resource.
	EventNotFound = "EVENT_NOT_FOUND"
)
