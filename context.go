package obscura

import (
	"context"
	"time"
)

// Context 返回指定 context 的克隆。
func (b *Browser) Context(ctx context.Context) *Browser {
	return &Browser{
		client:           b.client,
		ctx:              ctx,
		cancel:           b.cancel,
		launchCleanup:    b.launchCleanup,
		pages:            b.pages,
		pagesMu:          b.pagesMu, // 共享同一个 mutex
		BrowserContextID: b.BrowserContextID,
		timeout:          b.timeout,
		eventCh:          b.eventCh,
	}
}

// GetContext 返回当前 context。
func (b *Browser) GetContext() context.Context {
	return b.ctx
}

// Timeout 返回带超时的克隆。
func (b *Browser) Timeout(d time.Duration) *Browser {
	ctx, cancel := context.WithTimeout(b.ctx, d)
	newB := b.Context(ctx)
	newB.cancel = cancel
	return newB
}

// WithCancel 返回带 cancel 的克隆。
func (b *Browser) WithCancel() (*Browser, context.CancelFunc) {
	ctx, cancel := context.WithCancel(b.ctx)
	newB := b.Context(ctx)
	newB.cancel = cancel
	return newB, cancel
}

// Context 返回指定 context 的克隆。
func (p *Page) Context(ctx context.Context) *Page {
	p2 := *p
	p2.ctx = ctx
	return &p2
}

// GetContext 返回当前 context。
func (p *Page) GetContext() context.Context {
	return p.ctx
}

// Timeout 返回带超时的克隆。
func (p *Page) Timeout(d time.Duration) *Page {
	ctx, cancel := context.WithTimeout(p.ctx, d)
	newP := p.Context(ctx)
	newP.cancel = cancel
	return newP
}

// WithCancel 返回带 cancel 的克隆。
func (p *Page) WithCancel() (*Page, context.CancelFunc) {
	ctx, cancel := context.WithCancel(p.ctx)
	return p.Context(ctx), cancel
}

// Context 返回指定 context 的克隆。
func (el *Element) Context(ctx context.Context) *Element {
	el2 := *el
	el2.ctx = ctx
	return &el2
}

// GetContext 返回当前 context。
func (el *Element) GetContext() context.Context {
	return el.ctx
}
