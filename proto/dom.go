package proto

// DOM.getDocument
type DOMGetDocument struct {
	Depth int `json:"depth,omitempty"`
}
func (r DOMGetDocument) Method() string { return "DOM.getDocument" }

type DOMGetDocumentResult struct {
	Root *Node `json:"root"`
}

// DOM.querySelector
type DOMQuerySelector struct {
	NodeID   int    `json:"nodeId"`
	Selector string `json:"selector"`
}
func (r DOMQuerySelector) Method() string { return "DOM.querySelector" }

type DOMQuerySelectorResult struct {
	NodeID int `json:"nodeId"`
}

// DOM.querySelectorAll
type DOMQuerySelectorAll struct {
	NodeID   int    `json:"nodeId"`
	Selector string `json:"selector"`
}
func (r DOMQuerySelectorAll) Method() string { return "DOM.querySelectorAll" }

type DOMQuerySelectorAllResult struct {
	NodeIDs []int `json:"nodeIds"`
}

// DOM.getOuterHTML
type DOMGetOuterHTML struct {
	NodeID int `json:"nodeId"`
}
func (r DOMGetOuterHTML) Method() string { return "DOM.getOuterHTML" }

type DOMGetOuterHTMLResult struct {
	OuterHTML string `json:"outerHTML"`
}

// DOM.resolveNode
type DOMResolveNode struct {
	NodeID int `json:"nodeId"`
}
func (r DOMResolveNode) Method() string { return "DOM.resolveNode" }

type DOMResolveNodeResult struct {
	Object *RemoteObject `json:"object"`
}

// DOM.getBoxModel
type DOMGetBoxModel struct {
	NodeID int `json:"nodeId"`
}
func (r DOMGetBoxModel) Method() string { return "DOM.getBoxModel" }

type DOMGetBoxModelResult struct {
	Model *DOMBoxModel `json:"model"`
}

type DOMBoxModel struct {
	Content []float64 `json:"content"`
}
