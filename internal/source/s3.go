package source

// s3Source downloads an object from s3 into its working directory. The
// fetch mechanism is not implemented yet; Pull only reports the pending
// support.
type s3Source struct {
	dir string
	url string
}

// ensure to implement the Source interface
var _ Source = s3Source{}

func (s s3Source) Pull() error {
	return errNotImplemented
}
