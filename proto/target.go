package proto

// Target.createTarget
type TargetCreateTarget struct {
	URL    string `json:"url"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
}
func (r TargetCreateTarget) Method() string { return "Target.createTarget" }

type TargetCreateTargetResult struct {
	TargetID string `json:"targetId"`
}

// Target.closeTarget
type TargetCloseTarget struct {
	TargetID string `json:"targetId"`
}
func (r TargetCloseTarget) Method() string { return "Target.closeTarget" }

// Target.attachToTarget
type TargetAttachToTarget struct {
	TargetID string `json:"targetId"`
	Flatten  bool   `json:"flatten"`
}
func (r TargetAttachToTarget) Method() string { return "Target.attachToTarget" }

type TargetAttachToTargetResult struct {
	SessionID string `json:"sessionId"`
}

// Target.createBrowserContext
type TargetCreateBrowserContext struct{}
func (r TargetCreateBrowserContext) Method() string { return "Target.createBrowserContext" }

type TargetCreateBrowserContextResult struct {
	BrowserContextID string `json:"browserContextId"`
}

// Target.disposeBrowserContext
type TargetDisposeBrowserContext struct {
	BrowserContextID string `json:"browserContextId"`
}
func (r TargetDisposeBrowserContext) Method() string { return "Target.disposeBrowserContext" }

// Target.setDiscoverTargets
type TargetSetDiscoverTargets struct {
	Discover bool `json:"discover"`
}
func (r TargetSetDiscoverTargets) Method() string { return "Target.setDiscoverTargets" }

// Target.getTargets
type TargetGetTargets struct{}
func (r TargetGetTargets) Method() string { return "Target.getTargets" }

type TargetGetTargetsResult struct {
	TargetInfos []TargetInfo `json:"targetInfos"`
}

// Target.getTargetInfo
type TargetGetTargetInfo struct {
	TargetID string `json:"targetId"`
}
func (r TargetGetTargetInfo) Method() string { return "Target.getTargetInfo" }

type TargetGetTargetInfoResult struct {
	TargetInfo TargetInfo `json:"targetInfo"`
}

// TargetInfo
type TargetInfo struct {
	TargetID string `json:"targetId"`
	Type     string `json:"type"`
	Title    string `json:"title"`
	URL      string `json:"url"`
}
