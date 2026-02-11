package main

// HmeEmail represents a single Hide My Email alias.
type HmeEmail struct {
	Hme             string `json:"hme"`
	Label           string `json:"label"`
	Note            string `json:"note"`
	IsActive        bool   `json:"isActive"`
	CreateTimestamp int64  `json:"createTimestamp"`
	AnonymousID     string `json:"anonymousId"`
	ForwardToEmail  string `json:"forwardToEmail"`
	Origin          string `json:"origin"`
}

// GenerateResponse is the top-level response from the generate endpoint.
type GenerateResponse struct {
	Success bool       `json:"success"`
	Result  GenerateResult `json:"result"`
	Error   *APIError  `json:"error"`
}

// GenerateResult holds the generated email address.
type GenerateResult struct {
	Hme string `json:"hme"`
}

// ReserveRequest is the body sent to the reserve endpoint.
type ReserveRequest struct {
	Hme   string `json:"hme"`
	Label string `json:"label"`
	Note  string `json:"note"`
}

// ReserveResponse is the top-level response from the reserve endpoint.
type ReserveResponse struct {
	Success bool      `json:"success"`
	Error   *APIError `json:"error"`
}

// ListResponse is the top-level response from the list endpoint.
type ListResponse struct {
	Success bool       `json:"success"`
	Result  ListResult `json:"result"`
	Error   *APIError  `json:"error"`
}

// ListResult holds the list of HME aliases.
type ListResult struct {
	HmeEmails      []HmeEmail `json:"hmeEmails"`
	ForwardToEmails []string  `json:"forwardToEmails"`
}

// APIError represents an error returned by the iCloud API.
type APIError struct {
	ErrorCode    int    `json:"errorCode"`
	ErrorMessage string `json:"errorMessage"`
	Reason       string `json:"reason"`
}
