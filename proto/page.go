package proto

// Page.navigate
type PageNavigate struct {
	URL string `json:"url"`
}
func (r PageNavigate) Method() string { return "Page.navigate" }

type PageNavigateResult struct {
	FrameID   string `json:"frameId"`
	LoaderID  string `json:"loaderId"`
	ErrorText string `json:"errorText,omitempty"`
}

// Page.getFrameTree
type PageGetFrameTree struct{}
func (r PageGetFrameTree) Method() string { return "Page.getFrameTree" }

type PageGetFrameTreeResult struct {
	FrameTree *PageFrameTree `json:"frameTree"`
}

type PageFrameTree struct {
	Frame       *PageFrame       `json:"frame"`
	ChildFrames []*PageFrameTree `json:"childFrames,omitempty"`
}

type PageFrame struct {
	ID     string `json:"id"`
	Loader string `json:"loaderId"`
	URL    string `json:"url"`
}

// Page.enable
type PageEnable struct{}
func (r PageEnable) Method() string { return "Page.enable" }

// Page.addScriptToEvaluateOnNewDocument
type PageAddScriptToEvaluateOnNewDocument struct {
	Source string `json:"source"`
}
func (r PageAddScriptToEvaluateOnNewDocument) Method() string {
	return "Page.addScriptToEvaluateOnNewDocument"
}

type PageAddScriptToEvaluateOnNewDocumentResult struct {
	Identifier string `json:"identifier"`
}

// Page.setDeviceMetricsOverride
type PageSetDeviceMetricsOverride struct {
	Width             int     `json:"width"`
	Height            int     `json:"height"`
	DeviceScaleFactor float64 `json:"deviceScaleFactor"`
	Mobile            bool    `json:"mobile"`
}
func (r PageSetDeviceMetricsOverride) Method() string {
	return "Page.setDeviceMetricsOverride"
}

// Page.captureScreenshot
type PageCaptureScreenshot struct {
	Format string `json:"format,omitempty"`
}
func (r PageCaptureScreenshot) Method() string { return "Page.captureScreenshot" }

type PageCaptureScreenshotResult struct {
	Data string `json:"data"`
}

// 事件
type PageLoadEventFired struct {
	Timestamp float64 `json:"timestamp"`
}
func (e PageLoadEventFired) EventName() string { return "Page.loadEventFired" }

type PageDOMContentEventFired struct {
	Timestamp float64 `json:"timestamp"`
}
func (e PageDOMContentEventFired) EventName() string { return "Page.domContentEventFired" }

type PageFrameNavigated struct {
	Frame *PageFrame `json:"frame"`
}
func (e PageFrameNavigated) EventName() string { return "Page.frameNavigated" }
