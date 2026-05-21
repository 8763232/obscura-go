package proto

// LP.getMarkdown（Obscura 私有域）
type LPGetMarkdown struct{}
func (r LPGetMarkdown) Method() string { return "LP.getMarkdown" }

type LPGetMarkdownResult struct {
	Markdown string `json:"markdown"`
}
