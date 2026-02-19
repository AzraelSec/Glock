package serializer

import yaml "gopkg.in/yaml.v3"

type yamlDecoder struct{}

func (yd yamlDecoder) Marshal(in any) ([]byte, error) {
	return yaml.Marshal(in)
}

func (yd yamlDecoder) Unmarshal(in []byte, out any) error {
	return yaml.Unmarshal(in, out)
}

func NewYamlDecoder() yamlDecoder {
	return yamlDecoder{}
}
