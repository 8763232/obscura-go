package proto

// Runtime.evaluate
type RuntimeEvaluate struct {
	Expression    string `json:"expression"`
	ObjectGroup   string `json:"objectGroup,omitempty"`
	ReturnByValue bool   `json:"returnByValue,omitempty"`
}
func (r RuntimeEvaluate) Method() string { return "Runtime.evaluate" }

type RuntimeEvaluateResult struct {
	Result           *RemoteObject `json:"result"`
	ExceptionDetails any           `json:"exceptionDetails,omitempty"`
}

// Runtime.callFunctionOn
type RuntimeCallFunctionOn struct {
	FunctionDeclaration string          `json:"functionDeclaration"`
	ObjectID            string          `json:"objectId,omitempty"`
	Arguments           []*CallArgument `json:"arguments,omitempty"`
	ReturnByValue       bool            `json:"returnByValue"`
}
func (r RuntimeCallFunctionOn) Method() string { return "Runtime.callFunctionOn" }

type RuntimeCallFunctionOnResult struct {
	Result *RemoteObject `json:"result"`
}

type CallArgument struct {
	Value  any    `json:"value,omitempty"`
	Handle string `json:"handle,omitempty"`
}

// Runtime.getProperties
type RuntimeGetProperties struct {
	ObjectID    string `json:"objectId"`
	OwnOnly     bool   `json:"ownProperties,omitempty"`
	Accessors   bool   `json:"accessorPropertiesOnly,omitempty"`
}
func (r RuntimeGetProperties) Method() string { return "Runtime.getProperties" }

type RuntimeGetPropertiesResult struct {
	Result []*PropertyDescriptor `json:"result"`
}

type PropertyDescriptor struct {
	Name         string        `json:"name"`
	Value        *RemoteObject `json:"value,omitempty"`
	Writable     bool          `json:"writable"`
	Configurable bool          `json:"configurable"`
	Enumerable   bool          `json:"enumerable"`
}
