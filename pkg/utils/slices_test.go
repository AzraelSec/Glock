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
		for idx, item := range tt.want {
			if res[idx] != item {
				t.Errorf("unexpected item, want=%v got=%v", res[idx], item)
			}
		}
	}
}
