package utils

func Uniq[T comparable](list []T) []T {
	cmpMap := make(map[T]interface{})
	for _, el := range list {
		cmpMap[el] = struct{}{}
	}
	res := make([]T, 0)
	for k := range cmpMap {
		res = append(res, k)
	}
	return res
}

func Map[T, R any](list []T, handler func(i T) R) []R {
	res := make([]R, len(list))
	for idx, item := range list {
		res[idx] = handler(item)
	}
	return res
}
