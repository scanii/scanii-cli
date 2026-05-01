package client

// AccountInfo defines the account information response.
type AccountInfo struct {
	Balance          *float32           `json:"balance,omitempty"`
	BillingEmail     *string            `json:"billing_email,omitempty"`
	CreationDate     *string            `json:"creation_date,omitempty"`
	Keys             *map[string]APIKey `json:"keys,omitempty"`
	ModificationDate *string            `json:"modification_date,omitempty"`
	Name             *string            `json:"name,omitempty"`
	StartingBalance  *float32           `json:"starting_balance,omitempty"`
	Subscription     *string            `json:"subscription,omitempty"`
	Users            *map[string]User   `json:"users,omitempty"`
}

// APIKey defines an API key's properties.
type APIKey struct {
	Active                     *bool     `json:"active,omitempty"`
	CreationDate               *string   `json:"creation_date,omitempty"`
	DetectionCategoriesEnabled *[]string `json:"detection_categories_enabled,omitempty"`
	LastSeenDate               *string   `json:"last_seen_date,omitempty"`
	Tags                       *[]string `json:"tags,omitempty"`
}

// User defines a user account.
type User struct {
	CreationDate *string `json:"creation_date,omitempty"`
	LastLogin    *string `json:"last_login,omitempty"`
}

// AuthToken defines a temporary authentication token.
type AuthToken struct {
	CreationDate   *string `json:"creation_date,omitempty"`
	ExpirationDate *string `json:"expiration_date,omitempty"`
	ID             *string `json:"id,omitempty"`
}

// ProcessingResponse is the result of a file analysis.
type ProcessingResponse struct {
	Checksum      *string            `json:"checksum,omitempty"`
	ContentLength *float32           `json:"content_length,omitempty"`
	ContentType   *string            `json:"content_type,omitempty"`
	CreationDate  *string            `json:"creation_date,omitempty"`
	Error         *string            `json:"error,omitempty"`
	Findings      *[]string          `json:"findings,omitempty"`
	ID            *string            `json:"id,omitempty"`
	Metadata      *map[string]string `json:"metadata,omitempty"`
}

// ProcessingPendingResponse is the acknowledgment for an async processing request.
type ProcessingPendingResponse struct {
	ID *string `json:"id,omitempty"`
}

// ErrorResponse defines an API error.
type ErrorResponse struct {
	Error    *string            `json:"error,omitempty"`
	ID       *string            `json:"id,omitempty"`
	Metadata *map[string]string `json:"metadata,omitempty"`
}

// TraceEvent is a single event in a processing trace.
type TraceEvent struct {
	Message   *string `json:"message,omitempty"`
	Timestamp *string `json:"timestamp,omitempty"`
}

// TraceResponse is the ordered list of processing events for a given id.
type TraceResponse struct {
	Events *[]TraceEvent `json:"events,omitempty"`
	ID     *string       `json:"id,omitempty"`
}
