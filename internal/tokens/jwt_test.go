package tokens

import (
	"testing"
)

func TestGenerateTokenForNamespace(t *testing.T) {
	type args struct {
		secret        string
		namespaceName string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "Simple token gen test",
			args: args{
				secret:        "testsecret",
				namespaceName: "testNsName",
			},
			want:    "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJuYW1lc3BhY2UiOiJ0ZXN0TnNOYW1lIn0.jxNjQIaXtDeaU2_yRLssGXwe39YF38SPJqMDJ9UW63k",
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GenerateTokenForNamespace(tt.args.secret, tt.args.namespaceName)
			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateTokenForNamespace() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("GenerateTokenForNamespace() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestValidateAndExtractNamespaceFromToken(t *testing.T) {
	type args struct {
		key   string
		token string
	}
	tests := []struct {
		name    string
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "No errors, namespace testNsName",
			args: args{
				key:   "testsecret",
				token: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJuYW1lc3BhY2UiOiJ0ZXN0TnNOYW1lIn0.jxNjQIaXtDeaU2_yRLssGXwe39YF38SPJqMDJ9UW63k",
			},
			want:    "testNsName",
			wantErr: false,
		},
		{
			name: "Valid, but no namespace key in payload",
			args: args{
				key:   "testsecret",
				token: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJibGFoIjoiYmxhaCJ9.VDooXVSmmPEQFtgMrbB5_Besn62yj5b4IL3OBCi4d4I",
			},
			wantErr: true,
		},
		{
			name: "Invalid, payload right, but will fail because we're encoding with secret of 'sneaky'",
			args: args{
				key:   "testsecret", //we've actually encoded this with the key 'sneaky' - so should fail
				token: "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJuYW1lc3BhY2UiOiJ0ZXN0TnNOYW1lIn0.RGty06bhlBeXfG1EVvtuOqAVIIZEi5DcmKbxKuBQ7d4",
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidateAndExtractNamespaceFromToken(tt.args.key, tt.args.token)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAndExtractNamespaceFromToken() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ValidateAndExtractNamespaceFromToken() got = %v, want %v", got, tt.want)
			}
		})
	}
}
