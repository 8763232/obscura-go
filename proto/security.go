package proto

// Security.setIgnoreCertificateErrors
type SecuritySetIgnoreCertificateErrors struct {
	Ignore bool `json:"ignore"`
}

func (r SecuritySetIgnoreCertificateErrors) Method() string {
	return "Security.setIgnoreCertificateErrors"
}
