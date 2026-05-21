package obscura

import (
	"context"
	"fmt"

	"github.com/8763232/obscura-go/proto"
)

// Element 是 DOM 元素的操作句柄。
// 不支持 iframe 和 shadow DOM 操作。
type Element struct {
	page     *Page
	nodeID   int
	selector string
	ctx      context.Context
}

// Click 点击元素（先获取 boxModel 计算中心坐标）。
func (el *Element) Click(ctx context.Context) error {
	var boxRes proto.DOMGetBoxModelResult
	if err := el.page.browser.callResult(ctx, el.page.sessionID,
		proto.DOMGetBoxModel{NodeID: el.nodeID}, &boxRes); err != nil {
		return err
	}

	if boxRes.Model == nil || len(boxRes.Model.Content) < 8 {
		return ErrInvalidSelector
	}

	// Content quad: [x1,y1, x2,y2, x3,y3, x4,y4]
	// 中心点 = ((x1+x3)/2, (y1+y3)/2)
	cx := (boxRes.Model.Content[0] + boxRes.Model.Content[4]) / 2
	cy := (boxRes.Model.Content[1] + boxRes.Model.Content[5]) / 2

	for _, evt := range []proto.InputDispatchMouseEvent{
		{Type: "mouseMoved", X: cx, Y: cy},
		{Type: "mousePressed", X: cx, Y: cy, Button: "left", ClickCount: 1},
		{Type: "mouseReleased", X: cx, Y: cy, Button: "left", ClickCount: 1},
	} {
		if err := el.page.browser.call(ctx, el.page.sessionID, evt); err != nil {
			return err
		}
	}
	return nil
}

// Input 在元素中输入文本。
func (el *Element) Input(ctx context.Context, text string) error {
	for _, ch := range text {
		key := string(ch)
		if err := el.page.browser.call(ctx, el.page.sessionID, proto.InputDispatchKeyEvent{
			Type: "keyDown",
			Text: key,
		}); err != nil {
			return err
		}
		if err := el.page.browser.call(ctx, el.page.sessionID, proto.InputDispatchKeyEvent{
			Type: "keyUp",
			Text: key,
		}); err != nil {
			return err
		}
	}
	return nil
}

// Text 获取元素的文本内容。
func (el *Element) Text(ctx context.Context) (string, error) {
	var resolveRes proto.DOMResolveNodeResult
	if err := el.page.browser.callResult(ctx, el.page.sessionID,
		proto.DOMResolveNode{NodeID: el.nodeID}, &resolveRes); err != nil {
		return "", err
	}

	var propRes proto.RuntimeGetPropertiesResult
	if err := el.page.browser.callResult(ctx, el.page.sessionID,
		proto.RuntimeGetProperties{ObjectID: resolveRes.Object.ObjectID, OwnOnly: true},
		&propRes); err != nil {
		return "", err
	}

	// 查找 innerText 属性
	for _, prop := range propRes.Result {
		if prop.Name == "innerText" && prop.Value != nil {
			if s, ok := prop.Value.Value.(string); ok {
				return s, nil
			}
		}
	}
	return "", nil
}

// HTML 获取元素的 outerHTML。
func (el *Element) HTML(ctx context.Context) (string, error) {
	var res proto.DOMGetOuterHTMLResult
	if err := el.page.browser.callResult(ctx, el.page.sessionID,
		proto.DOMGetOuterHTML{NodeID: el.nodeID}, &res); err != nil {
		return "", err
	}
	return res.OuterHTML, nil
}

// Attribute 获取元素的属性值。
func (el *Element) Attribute(ctx context.Context, name string) (string, error) {
	var resolveRes proto.DOMResolveNodeResult
	if err := el.page.browser.callResult(ctx, el.page.sessionID,
		proto.DOMResolveNode{NodeID: el.nodeID}, &resolveRes); err != nil {
		return "", err
	}

	// 通过 JS 获取属性
	js := fmt.Sprintf("function() { return this.getAttribute('%s'); }", name)
	var callRes proto.RuntimeCallFunctionOnResult
	if err := el.page.browser.callResult(ctx, el.page.sessionID, proto.RuntimeCallFunctionOn{
		FunctionDeclaration: js,
		ObjectID:            resolveRes.Object.ObjectID,
		ReturnByValue:       true,
	}, &callRes); err != nil {
		return "", err
	}

	if callRes.Result != nil && callRes.Result.Value != nil {
		if s, ok := callRes.Result.Value.(string); ok {
			return s, nil
		}
	}
	return "", nil
}
