package postprocess

import (
	"lagoon.sh/insights-remote/internal"
	"reflect"
	"testing"
)

func TestDependencyTrackPostProcess_processTemplate(t *testing.T) {
	type fields struct {
		ApiEndpoint               string
		ApiKey                    string
		RootProjectName           string
		ParentProjectNameTemplate string
		ProjectNameTemplate       string
	}
	type args struct {
		info dependencyTrackWriteInfo
	}
	tests := []struct {
		name    string
		fields  fields
		args    args
		want    string
		wantErr bool
	}{
		{
			name: "simple passing test",
			fields: fields{
				ParentProjectNameTemplate: "ParentProjectName",
			},
			args: args{
				info: dependencyTrackWriteInfo{
					ParentProjectName: "ParentProjectName",
				},
			},
			want:    "ParentProjectName",
			wantErr: false,
		},
		{
			name: "template test",
			fields: fields{
				ParentProjectNameTemplate: "{{.ParentProjectName}}",
			},
			args: args{
				info: dependencyTrackWriteInfo{
					ParentProjectName: "ThisIsTheParentProjectName",
				},
			},
			want:    "ThisIsTheParentProjectName",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &DependencyTrackPostProcess{
				ApiEndpoint: tt.fields.ApiEndpoint,
				ApiKey:      tt.fields.ApiKey,
				Templates: DependencyTrackTemplates{
					RootProjectNameTemplate:   tt.fields.RootProjectName,
					ParentProjectNameTemplate: tt.fields.ParentProjectNameTemplate,
					ProjectNameTemplate:       tt.fields.ProjectNameTemplate,
				},
			}
			got, err := processTemplate(d.Templates.ParentProjectNameTemplate, tt.args.info)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetParentProjectName() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("GetParentProjectName() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getWriteInfo(t *testing.T) {
	type args struct {
		message   internal.LagoonInsightsMessage
		templates DependencyTrackTemplates
	}
	tests := []struct {
		name    string
		args    args
		want    dependencyTrackWriteInfo
		wantErr bool
	}{
		{
			name: "Project, no parents",
			args: args{
				message: internal.LagoonInsightsMessage{
					Labels: map[string]string{
						"lagoon.sh/project":     "testprojectlabel",
						"lagoon.sh/environment": "testenvironmentlabel",
						"lagoon.sh/service":     "clilabel",
					},
					Project:     "testProject",
					Environment: "testEnvironment",
				},
				templates: DependencyTrackTemplates{
					RootProjectNameTemplate:   "",
					ParentProjectNameTemplate: "",
					ProjectNameTemplate:       "{{ .ProjectName }}-{{ .ServiceName }}",
					VersionTemplate:           "1.0.0",
				},
			},
			want: dependencyTrackWriteInfo{
				ParentProjectName: "",
				ProjectName:       "testProject-clilabel",
				ProjectVersion:    "1.0.0",
			},
		},
		{
			name: "Project with parent",
			args: args{
				message: internal.LagoonInsightsMessage{
					Labels: map[string]string{
						"lagoon.sh/project":     "testprojectlabel",
						"lagoon.sh/environment": "testenvironmentlabel",
						"lagoon.sh/service":     "clilabel",
					},
					Project:     "testProject",
					Environment: "testEnvironment",
				},
				templates: DependencyTrackTemplates{
					RootProjectNameTemplate:   "",
					ParentProjectNameTemplate: "testproject-{{ .ProjectName }}",
					ProjectNameTemplate:       "{{ .ProjectName }}-{{ .ServiceName }}",
					VersionTemplate:           "1.0.0",
				},
			},
			want: dependencyTrackWriteInfo{
				ParentProjectName: "testproject-testProject",
				ProjectName:       "testProject-clilabel",
				ProjectVersion:    "1.0.0",
			},
		},
		{
			name: "Project with root and parent",
			args: args{
				message: internal.LagoonInsightsMessage{
					Labels: map[string]string{
						"lagoon.sh/project":     "testprojectlabel",
						"lagoon.sh/environment": "testenvironmentlabel",
						"lagoon.sh/service":     "clilabel",
					},
					Project:     "testProject",
					Environment: "testEnvironment",
				},
				templates: DependencyTrackTemplates{
					RootProjectNameTemplate:   "SomeRootProject",
					ParentProjectNameTemplate: "testproject-{{ .ProjectName }}",
					ProjectNameTemplate:       "{{ .ProjectName }}-{{ .ServiceName }}",
					VersionTemplate:           "1.0.0",
				},
			},

			want: dependencyTrackWriteInfo{
				ParentProjectName: "testproject-testProject",
				ProjectName:       "testProject-clilabel",
				ProjectVersion:    "1.0.0",
			},
		},
		{
			name: "Project falling back to labels",
			args: args{
				message: internal.LagoonInsightsMessage{
					Labels: map[string]string{
						"lagoon.sh/project":     "testprojectlabel",
						"lagoon.sh/environment": "testenvironmentlabel",
						"lagoon.sh/service":     "clilabel",
					},
					Project:     "",
					Environment: "testEnvironment",
				},
				templates: DependencyTrackTemplates{
					RootProjectNameTemplate:   "SomeRootProject",
					ParentProjectNameTemplate: "testproject-{{ .ProjectName }}",
					ProjectNameTemplate:       "{{ .ProjectName }}-{{ .ServiceName }}",
					VersionTemplate:           "1.0.0",
				},
			},

			want: dependencyTrackWriteInfo{
				ParentProjectName: "testproject-testprojectlabel",
				ProjectName:       "testprojectlabel-clilabel",
				ProjectVersion:    "1.0.0",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getWriteInfo(tt.args.message, tt.args.templates)
			if (err != nil) != tt.wantErr {
				t.Errorf("getWriteInfo() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getWriteInfo() got = %v, want %v", got, tt.want)
			}
		})
	}
}
