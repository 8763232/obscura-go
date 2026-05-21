package proto

// Fetch.enable
type FetchEnable struct {
	Patterns           []*FetchRequestPattern `json:"patterns,omitempty"`
	HandleAuthRequests bool                   `json:"handleAuthRequests,omitempty"`
}
func (r FetchEnable) Method() string { return "Fetch.enable" }

type FetchRequestPattern struct {
	URLPattern   string `json:"urlPattern,omitempty"`
	ResourceType string `json:"resourceType,omitempty"`
	RequestStage string `json:"requestStage,omitempty"`
}

// Fetch.disable
type FetchDisable struct{}
func (r FetchDisable) Method() string { return "Fetch.disable" }

// Fetch.continueRequest
type FetchContinueRequest struct {
	RequestID string             `json:"requestId"`
	URL       string             `json:"url,omitempty"`
	HTTPMethod string            `json:"method,omitempty"`
	Headers   []FetchHeaderEntry `json:"headers,omitempty"`
	PostData  string             `json:"postData,omitempty"`
}
func (r FetchContinueRequest) Method() string { return "Fetch.continueRequest" }

type FetchHeaderEntry struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Fetch.fulfillRequest
type FetchFulfillRequest struct {
	RequestID       string             `json:"requestId"`
	ResponseCode    int                `json:"responseCode"`
	ResponseHeaders []FetchHeaderEntry `json:"responseHeaders,omitempty"`
	Body            string             `json:"body,omitempty"`
}
func (r FetchFulfillRequest) Method() string { return "Fetch.fulfillRequest" }

// Fetch.failRequest
type FetchFailRequest struct {
	RequestID   string `json:"requestId"`
	ErrorReason string `json:"errorReason"`
}
func (r FetchFailRequest) Method() string { return "Fetch.failRequest" }

// Fetch.requestPaused 事件
type FetchRequestPaused struct {
	RequestID          string             `json:"requestId"`
	Request            *FetchRequest      `json:"request"`
	ResourceType       string             `json:"resourceType"`
	ResponseStatusCode int                `json:"responseStatusCode,omitempty"`
	ResponseHeaders    []FetchHeaderEntry `json:"responseHeaders,omitempty"`
}
func (e FetchRequestPaused) EventName() string { return "Fetch.requestPaused" }

type FetchRequest struct {
	URL      string            `json:"url"`
	Method   string            `json:"method"`
	Headers  map[string]string `json:"headers"`
	PostData string            `json:"postData,omitempty"`
}
