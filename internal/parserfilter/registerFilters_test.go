package parserfilter

import (
	"reflect"
	"testing"
)

func TestLoadTransformsFromDisk(t *testing.T) {
	type args struct {
		filename string
	}
	tests := []struct {
		name    string
		args    args
		want    []FactTransform
		wantErr bool
	}{
		{
			name: "test1 - json",
			args: args{filename: "testassets/testLoadTransformsFromDisk/test1.json"},
			want: []FactTransform{
				{
					Type: "test1",
				},
			},
		},
		{
			name: "test2 - yaml",
			args: args{filename: "testassets/testLoadTransformsFromDisk/test2.yaml"},
			want: []FactTransform{
				{
					Type: "test2",
				},
			},
		},
		{
			name: "test3 - json",
			args: args{filename: "testassets/testLoadTransformsFromDisk/test3.yml"},
			want: []FactTransform{
				{
					Type: "test3",
				},
			},
		},
		{
			name:    "test4 - unsupported file type",
			args:    args{filename: "testassets/testLoadTransformsFromDisk/test4.unsp"},
			want:    []FactTransform{},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := LoadTransformsFromDisk(tt.args.filename)
			if (err != nil) != tt.wantErr {
				t.Errorf("LoadTransformsFromDisk() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) && tt.wantErr == false {
				t.Errorf("LoadTransformsFromDisk() got = %v, want %v", got, tt.want)
			}
		})
	}
}
