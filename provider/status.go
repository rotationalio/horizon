package provider

//go:generate enumify -names statusNames

type Status uint8

const (
	StatusUnknown Status = iota
	StatusPending
	StatusDisabled
	StatusDisconnected
	StatusInitializing
	StatusConnected
	StatusError
	StatusAuthRequired
	StatusRegRequired
)

var statusNames = [][]string{
	{
		"unknown",
		"pending",
		"disabled",
		"disconnected",
		"initializing",
		"connected",
		"error",
		"auth_required",
		"reg_required",
	},
}

// StatusNames returns the names of the statuses.
func StatusNames() []string {
	return statusNames[0]
}
