package serializer

import "encoding/json"

type jsonDecoder struct{}

func (jd jsonDecoder) Marshal(in interface{}) ([]byte, error) {
	return json.Marshal(in)
}

func (jd jsonDecoder) Unmarshal(in []byte, out interface{}) error {
	return json.Unmarshal(in, out)
}

func NewJsonDecoder() jsonDecoder {
	return jsonDecoder{}
}
