package cmd

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseSelection(t *testing.T) {
	tests := []struct {
		name    string
		specs   []string
		n       int
		want    []int
		wantErr string
	}{
		{"comma list", []string{"1,3"}, 5, []int{1, 3}, ""},
		{"split by pflag", []string{"1", "3"}, 5, []int{1, 3}, ""},
		{"order kept", []string{"3", "1"}, 5, []int{3, 1}, ""},
		{"range", []string{"2-4"}, 5, []int{2, 3, 4}, ""},
		{"single-number range", []string{"2-2"}, 5, []int{2}, ""},
		{"all", []string{"all"}, 3, []int{1, 2, 3}, ""},
		{"all case-insensitive", []string{"ALL"}, 2, []int{1, 2}, ""},
		{"all with no results", []string{"all"}, 0, nil, ""},
		{"duplicates removed", []string{"1,1,2"}, 5, []int{1, 2}, ""},
		{"overlap keeps first", []string{"3", "1-4"}, 5, []int{3, 1, 2, 4}, ""},
		{"mixed", []string{"1,3-4"}, 5, []int{1, 3, 4}, ""},
		{"spaces", []string{" 1 , 2 - 3 "}, 5, []int{1, 2, 3}, ""},
		{"zero", []string{"0"}, 5, nil, `--get "0": expected a result number`},
		{"past end", []string{"6"}, 5, nil, "--get 6: no such result (have 1-5)"},
		{"range past end", []string{"4-6"}, 5, nil, "--get 6: no such result (have 1-5)"},
		{"reversed range", []string{"3-1"}, 5, nil, "range is backwards"},
		{"garbage", []string{"abc"}, 5, nil, `--get "abc": expected a result number`},
		{"open range", []string{"1-"}, 5, nil, `--get "1-": expected a result number`},
		{"negative", []string{"-1"}, 5, nil, `--get "-1": expected a result number`},
		{"empty item", []string{"1,,2"}, 5, nil, `--get "": expected a result number`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSelection(tt.specs, tt.n)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}
