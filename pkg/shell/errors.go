package shell

import "strings"

func IgnoreInterrupt(e error) error {
	if e == nil {
		return e
	}
	if strings.Contains(e.Error(), "signal: interrupt") {
		return nil
	}
	if strings.Contains(e.Error(), "signal: killed") {
		return nil
	}
	return e
}
