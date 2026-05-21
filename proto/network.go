package proto

// Network.enable
type NetworkEnable struct{}
func (r NetworkEnable) Method() string { return "Network.enable" }

// Network.setCookies
type NetworkSetCookies struct {
	Cookies []*CookieParam `json:"cookies"`
}
func (r NetworkSetCookies) Method() string { return "Network.setCookies" }

type CookieParam struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain,omitempty"`
	Path     string  `json:"path,omitempty"`
	Secure   bool    `json:"secure,omitempty"`
	HTTPOnly bool    `json:"httpOnly,omitempty"`
	SameSite string  `json:"sameSite,omitempty"`
	Expires  float64 `json:"expires,omitempty"`
	URL      string  `json:"url,omitempty"`
}

type Cookie struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain"`
	Path     string  `json:"path"`
	Secure   bool    `json:"secure"`
	HTTPOnly bool    `json:"httpOnly"`
	SameSite string  `json:"sameSite,omitempty"`
	Expires  float64 `json:"expires"`
}

// Network.getCookies
type NetworkGetCookies struct {
	Urls []string `json:"urls,omitempty"`
}
func (r NetworkGetCookies) Method() string { return "Network.getCookies" }

type NetworkGetCookiesResult struct {
	Cookies []*Cookie `json:"cookies"`
}

// Network.setExtraHTTPHeaders
type NetworkSetExtraHTTPHeaders struct {
	Headers map[string]string `json:"headers"`
}
func (r NetworkSetExtraHTTPHeaders) Method() string { return "Network.setExtraHTTPHeaders" }

// Network.setUserAgentOverride
type NetworkSetUserAgentOverride struct {
	UserAgent string `json:"userAgent"`
}
func (r NetworkSetUserAgentOverride) Method() string {
	return "Network.setUserAgentOverride"
}
