package proto

// Storage.getCookies
type StorageGetCookies struct {
	BrowserContextID string `json:"browserContextId,omitempty"`
}
func (r StorageGetCookies) Method() string { return "Storage.getCookies" }

type StorageGetCookiesResult struct {
	Cookies []*Cookie `json:"cookies"`
}

// Storage.setCookies
type StorageSetCookies struct {
	Cookies          []*CookieParam `json:"cookies"`
	BrowserContextID string         `json:"browserContextId,omitempty"`
}
func (r StorageSetCookies) Method() string { return "Storage.setCookies" }

// Storage.clearCookies
type StorageClearCookies struct {
	BrowserContextID string `json:"browserContextId,omitempty"`
}
func (r StorageClearCookies) Method() string { return "Storage.clearCookies" }

// Storage.deleteCookies
type StorageDeleteCookies struct {
	Name    string `json:"name"`
	URL     string `json:"url,omitempty"`
	Domain  string `json:"domain,omitempty"`
	Path    string `json:"path,omitempty"`
}
func (r StorageDeleteCookies) Method() string { return "Storage.deleteCookies" }
