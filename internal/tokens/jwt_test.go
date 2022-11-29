package tokens

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestGenerateTokenForNamespace(t *testing.T) {
	type args struct {
		secret  string
		details NamespaceDetails
	}
	tests := []struct {
		name          string
		args          args
		want          NamespaceDetails
		wantErr       bool
		wantCompError bool
	}{
		{
			name: "simple passing test",
			args: args{
				secret: "testing",
				details: NamespaceDetails{
					Namespace:       "Namespace",
					EnvironmentId:   "1",
					ProjectName:     "Project",
					EnvironmentName: "Environment",
				},
			},
			want: NamespaceDetails{
				Namespace:       "Namespace",
				EnvironmentId:   "1",
				ProjectName:     "Project",
				EnvironmentName: "Environment",
			},
		},
		{
			name: "simple failing test",
			args: args{
				secret: "testing",
				details: NamespaceDetails{
					Namespace:       "Namespace",
					EnvironmentId:   "1",
					ProjectName:     "Project",
					EnvironmentName: "Environment",
				},
			},
			want: NamespaceDetails{
				EnvironmentId:   "1",
				ProjectName:     "nope",
				EnvironmentName: "nope",
			},
			wantCompError: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			token, err := GenerateTokenForNamespace(tt.args.secret, tt.args.details)
			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateTokenForNamespace() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			details, err := ValidateAndExtractNamespaceDetailsFromToken(tt.args.secret, token)
			require.NoError(t, err)
			if details.EnvironmentId != tt.args.details.EnvironmentId {
				if !tt.wantCompError {
					t.Errorf("GenerateTokenForNamespace() got = %v, want %v", details.EnvironmentId, tt.args.details.EnvironmentId)
				}
			}

			if details.ProjectName != tt.args.details.ProjectName {
				if !tt.wantCompError {
					t.Errorf("GenerateTokenForNamespace() got = %v, want %v", details.ProjectName, tt.args.details.ProjectName)
				}
			}

			if details.EnvironmentName != tt.args.details.EnvironmentName {
				if !tt.wantCompError {
					t.Errorf("GenerateTokenForNamespace() got = %v, want %v", details.EnvironmentName, tt.args.details.EnvironmentName)
				}
			}

		})
	}
}
