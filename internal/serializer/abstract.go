package serializer

type Serializer interface {
	Marshal(in any) (out []byte, err error)
	Unmarshal(in []byte, out any) (err error)
}
