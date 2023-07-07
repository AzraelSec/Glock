package utils

func Truthy(v interface{}) bool {
	if v, ok := v.(string); ok {
		return v != ""
	}
	if v, ok := v.(bool); ok {
		return v
	}
	if v, ok := v.(int); ok {
		return v != 0
	}
	return v != nil
}
