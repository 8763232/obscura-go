package proto

// Request 是所有 CDP 请求类型必须实现的接口。
type Request interface {
	Method() string
}

// Event 是所有 CDP 事件类型必须实现的接口。
type Event interface {
	EventName() string
}

// Node 是 DOM 节点类型。
type Node struct {
	NodeID    int     `json:"nodeId"`
	NodeType  int     `json:"nodeType"`
	NodeName  string  `json:"nodeName"`
	NodeValue string  `json:"nodeValue"`
	Children  []*Node `json:"children,omitempty"`
}

// RemoteObject 是 Runtime 远程对象。
type RemoteObject struct {
	Type        string         `json:"type"`
	Subtype     string         `json:"subtype,omitempty"`
	ClassName   string         `json:"className,omitempty"`
	Value       any            `json:"value,omitempty"`
	ObjectID    string         `json:"objectId,omitempty"`
	Description string         `json:"description,omitempty"`
	Preview     *ObjectPreview `json:"preview,omitempty"`
}

// ObjectPreview 是远程对象的预览。
type ObjectPreview struct {
	Type        string              `json:"type"`
	Subtype     string              `json:"subtype"`
	Description string              `json:"description"`
	Properties  []*PropertyPreview  `json:"properties"`
}

// PropertyPreview 是属性预览。
type PropertyPreview struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
}
