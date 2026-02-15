package errors

// Error code constants for API responses.
const (
	// Network errors
	ErrNetworkNotFound       = "NETWORK_NOT_FOUND"
	ErrNetworkAlreadyExists  = "NETWORK_ALREADY_EXISTS"
	ErrInterfaceCreateFailed = "INTERFACE_CREATE_FAILED"
	ErrInterfaceUpFailed     = "INTERFACE_UP_FAILED"
	ErrSubnetConflict        = "SUBNET_CONFLICT"
	ErrPortInUse             = "PORT_IN_USE"

	// Peer errors
	ErrPeerNotFound      = "PEER_NOT_FOUND"
	ErrPeerAlreadyExists = "PEER_ALREADY_EXISTS"
	ErrPeerAddFailed     = "WG_PEER_ADD_FAILED"
	ErrIPExhausted       = "IP_POOL_EXHAUSTED"
	ErrInvalidAllowedIPs = "INVALID_ALLOWED_IPS"

	// Auth errors
	ErrForbidden          = "FORBIDDEN"
	ErrUnauthorized       = "UNAUTHORIZED"
	ErrSessionExpired     = "SESSION_EXPIRED"
	ErrInvalidCredentials = "INVALID_CREDENTIALS"
	ErrSetupComplete      = "SETUP_ALREADY_COMPLETE"
	ErrSetupRequired      = "SETUP_REQUIRED"
	ErrStepOrderViolation = "STEP_ORDER_VIOLATION"
	ErrInvalidOTP         = "INVALID_OTP"
	ErrRateLimited        = "RATE_LIMITED"

	// System errors
	ErrWGModuleNotLoaded   = "WG_MODULE_NOT_LOADED"
	ErrCapabilityMissing   = "CAPABILITY_MISSING"
	ErrNFTablesUnavailable = "NFTABLES_UNAVAILABLE"
	ErrDatabaseCorrupted   = "DATABASE_CORRUPTED"

	// Bridge errors
	ErrBridgeNotFound      = "BRIDGE_NOT_FOUND"
	ErrBridgeAlreadyExists = "BRIDGE_ALREADY_EXISTS"
	ErrBridgeSelfReference = "BRIDGE_SELF_REFERENCE"

	// Alert errors
	ErrAlertNotFound = "ALERT_NOT_FOUND"

	// General
	ErrValidation = "VALIDATION_ERROR"
	ErrInternal   = "INTERNAL_ERROR"
)
