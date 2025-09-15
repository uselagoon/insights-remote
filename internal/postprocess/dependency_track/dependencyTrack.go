package deptrack

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"text/template"

	dtrack "github.com/DependencyTrack/client-go"
	"lagoon.sh/insights-remote/internal"
)

type Templates struct {
	//RootProjectNameTemplate   string // If a root project is set, all subsequent projects will be children of this project
	//ParentProjectNameTemplate string
	ParentProjectNameTemplates []string
	ProjectNameTemplate        string
	VersionTemplate            string
}

type writeInfo struct {
	//RootName          string
	ParentProjectNames []string
	ProjectName        string
	ProjectVersion     string
}

// This is a helper function to process a template string given a dependencyTrackWriteInfo struct
func processTemplate(templateString string, info interface{}) (string, error) {
	tmpl, err := template.New("templateString").Parse(templateString)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, info)
	if err != nil {
		return "", err
	}
	return buf.String(), nil
}

// Given a LagoonInsights message, we do a best effort to extract the necessary information to write to DependencyTrack
func getWriteInfo(message internal.LagoonInsightsMessage, templates Templates) (writeInfo, error) {

	writeinfo := writeInfo{}

	// These are going to be what's available for templating
	templateValues := struct {
		ServiceName     string
		EnvironmentType string
		ProjectName     string
		EnvironmentName string
		Version         string
	}{}

	if message.Project == "" {
		if val, ok := message.Labels["lagoon.sh/project"]; ok {
			templateValues.ProjectName = val
		} else {
			return writeinfo, fmt.Errorf("no project name found - unable to populate info for dependency track")
		}
	} else {
		templateValues.ProjectName = message.Project
	}

	if message.Environment == "" {
		if val, ok := message.Labels["lagoon.sh/environment"]; ok {
			templateValues.EnvironmentName = val
		} else {
			return writeinfo, fmt.Errorf("no environment name found - unable to populate info for dependency track")
		}
	} else {
		templateValues.EnvironmentName = message.Environment
	}

	if val, ok := message.Labels["lagoon.sh/service"]; ok {
		templateValues.ServiceName = val
	} else {
		return writeinfo, fmt.Errorf("no service annotation found - unable to populate info for dependency track")
	}

	if val, ok := message.Labels["lagoon.sh/environmentType"]; ok {
		templateValues.EnvironmentType = val
	} else {
		templateValues.EnvironmentType = "unknown"
	}

	for _, parentProjectNameTemplate := range templates.ParentProjectNameTemplates {
		n, err := processTemplate(parentProjectNameTemplate, templateValues)
		if err != nil {
			return writeinfo, err
		}
		writeinfo.ParentProjectNames = append(writeinfo.ParentProjectNames, n)
	}

	n, err := processTemplate(templates.ProjectNameTemplate, templateValues)
	if err != nil {
		return writeinfo, err
	}
	writeinfo.ProjectName = n

	n, err = processTemplate(templates.VersionTemplate, templateValues)
	if err != nil {
		return writeinfo, err
	}
	writeinfo.ProjectVersion = n

	return writeinfo, nil
}

// This will get or create a project in DependencyTrack
func getOrCreateProject(client *dtrack.Client, projectName string, parentProject *dtrack.ParentRef) (dtrack.Project, error) {
	// let's ensure we have a parent project
	var project dtrack.Project
	projects, err := client.Project.GetProjectsForName(context.TODO(), projectName, true, false)
	if err != nil {
		return dtrack.Project{}, err
	}

	// let's create the project if it doesn't exist
	if len(projects) == 0 {
		project, err = client.Project.Create(context.TODO(), dtrack.Project{
			Name:          projectName,
			Active:        true,
			ParentRef:     parentProject,
			LastBOMImport: 0,
		})

		if err != nil {
			return dtrack.Project{}, err
		}
	} else {
		// if there's a parent project, we check which project has it
		if parentProject != nil {
			for _, project = range projects {
				// we need to get the full project object to check the parent
				fullProject, err := client.Project.Get(context.TODO(), project.UUID)
				if err != nil {
					return dtrack.Project{}, err
				}
				if fullProject.ParentRef != nil && fullProject.ParentRef.UUID == parentProject.UUID {
					return fullProject, nil
				}
			}
			// if we get here, something is wrong
			return dtrack.Project{}, fmt.Errorf("parent project %s not found for %s", parentProject.UUID, projectName)
		}
		// else we just take the first project
		project = projects[0]
	}
	return project, err
}

// Helper function to unzip a byte stream
func unzipByteStream(input io.Reader, output io.Writer) error {
	gzipReader, err := gzip.NewReader(input)
	if err != nil {
		return err
	}
	defer gzipReader.Close()

	_, err = io.Copy(output, gzipReader)
	return err
}
