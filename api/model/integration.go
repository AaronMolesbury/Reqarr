package model

// Integration results describe the most recent calls, not continuous health.
const (
	IntegrationSuccess       = "success"
	IntegrationPartial       = "partial"
	IntegrationError         = "error"
	IntegrationNotConfigured = "not_configured"
	IntegrationUnknown       = "unknown"
)
