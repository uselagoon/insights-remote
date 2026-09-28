package parserfilter

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"testing"

	LagoonFact "lagoon.sh/insights-remote/internal"
)

func Test_processFactsFromImageInspect(t *testing.T) {
	type args struct {
		logger                 *slog.Logger
		imageInspectDataSource string
		id                     string
		source                 string
	}
	tests := []struct {
		name     string
		args     args
		contains []LagoonFact.Fact
		wantErr  bool
	}{
		{
			name: "Testing Environment Variables",
			args: args{
				logger:                 slog.Default(),
				imageInspectDataSource: "testassets/imageInspectParserFilter/test1_envtesting.json",
				id:                     "0",
				source:                 "service",
			},
			contains: []LagoonFact.Fact{
				{
					Name:  "PYTHON_PIP_VERSION",
					Value: "1",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			imageInspectBytes, err := os.ReadFile(tt.args.imageInspectDataSource)
			if err != nil {
				slog.Error("Failed opening test file")
				panic(1)
			}

			imageInspectData := ImageData{}

			err = json.Unmarshal(imageInspectBytes, &imageInspectData)
			if err != nil {
				slog.Error(err.Error())
				panic(1)
			}

			got, err := processFactsFromImageInspect(tt.args.logger, imageInspectData, tt.args.id, tt.args.source)
			if (err != nil) != tt.wantErr {
				t.Errorf("processFactsFromImageInspect() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			fmt.Println(got)

			for _, v := range tt.contains {
				found := false
				for _, f := range got {
					if v.Name == f.Name && v.Value == f.Value {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("processFactsFromImageInspect() could not find target values in data loaded from file")
				}
			}
		})
	}
}
