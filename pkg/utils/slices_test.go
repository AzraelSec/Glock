package utils

import "testing"

func printUniqueValue[T comparable](arr []T) map[T]int {
	occs := make(map[T]int)
	for _, num := range arr {
		occs[num] = occs[num] + 1
	}
	return occs
}

func TestUniq(t *testing.T) {
	tests := []struct {
		in   []any
		want []any
	}{
		{
			in:   []any{"a", "b", "b", "a"},
			want: []any{"a", "b"},
		},
		{
			in:   []any{1, 2, 3, 4, 3, 2},
			want: []any{1, 2, 3, 4},
		},
	}
	for _, tt := range tests {
		res := Uniq(tt.in)
		occs := printUniqueValue(res)

		for _, item := range tt.want {
			if occs[item] != 1 {
				t.Errorf("unexpected number of occurrences for %v, want=1 got=%v", item, occs[item])
			}
		}
	}
}

func TestMap(t *testing.T) {
	tests := []struct {
		in      []any
		want    []any
		handler func(any) any
	}{
		{
			in:   []any{"a", "b", "b", "a"},
			want: []any{"a", "a", "a", "a"},
			handler: func(a any) any {
				return "a"
			},
		},
		{
			in:   []any{1, 2, 3, 4},
			want: []any{10, 20, 30, 40},
			handler: func(a any) any {
				return a.(int) * 10
			},
		},
	}
	for _, tt := range tests {
		res := Map(tt.in, tt.handler)
		for idx, item := range tt.want {
			if res[idx] != item {
				t.Errorf("unexpected item, want=%v got=%v", item, res[idx])
			}
		}
	}
}
