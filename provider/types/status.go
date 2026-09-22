package types

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
	// Names (parse / string representation)
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
	// Pill badge style tokens (without "badge rounded-pill " prefix; added in BadgeColor)
	{
		"badge-subtle-secondary",
		"badge-subtle-info",
		"badge-subtle-secondary",
		"badge-subtle-secondary",
		"badge-subtle-warning",
		"badge-subtle-success",
		"badge-subtle-danger",
		"badge-subtle-warning",
		"badge-subtle-warning",
	},
	// Font Awesome icon classes (full class strings; far vs fas matches bundled FA subset)
	{
		"far fa-fw fa-question-circle",
		"fas fa-fw fa-hourglass-start",
		"fas fa-fw fa-ban",
		"fas fa-fw fa-plug",
		"fas fa-fw fa-spinner",
		"far fa-fw fa-check-circle",
		"far fa-fw fa-circle-xmark",
		"fas fa-fw fa-key",
		"far fa-fw fa-id-card",
	},
	// English UI labels ([IntegrationStatus.Label])
	{
		"Unknown Status",
		"Pending",
		"Disabled",
		"Disconnected",
		"Initializing",
		"Connected",
		"Error Connecting",
		"Sign-In Required",
		"Application Registration Required",
	},
}

// BadgeColor returns the CSS class for the badge display of this IntegrationStatus.
func (s Status) BadgeColor() string {
	if s >= Status(len(statusNames[1])) {
		return "badge rounded-pill " + statusNames[1][0]
	}
	return "badge rounded-pill " + statusNames[1][s]
}

// Icon returns the Font Awesome icon class for this IntegrationStatus.
func (s Status) Icon() string {
	if s >= Status(len(statusNames[2])) {
		return statusNames[2][0]
	}
	return statusNames[2][s]
}

// Label returns a short English phrase for displaying this IntegrationStatus in the UI.
// Labels live in integrationStatusNames[3]; [Status.String] (row 0) remains the stable machine representation.
func (s Status) Label() string {
	row := statusNames[3]
	if s >= Status(len(row)) {
		return row[StatusUnknown]
	}
	return row[s]
}

// StatusNames returns the names of the statuses.
func StatusNames() []string {
	return statusNames[0]
}
