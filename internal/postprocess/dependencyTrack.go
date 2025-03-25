package postprocess

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"fmt"
	dtrack "github.com/DependencyTrack/client-go"
	"io"
	"lagoon.sh/insights-remote/internal"
	"text/template"
	"time"
)

type DependencyTrackTemplates struct {
	RootProjectNameTemplate   string // If a root project is set, all subsequent projects will be children of this project
	ParentProjectNameTemplate string
	ProjectNameTemplate       string
	VersionTemplate           string
}

type DependencyTrackPostProcess struct {
	ApiEndpoint string
	ApiKey      string
	Templates   DependencyTrackTemplates
}

type dependencyTrackWriteInfo struct {
	//RootName          string
	ParentProjectName string
	ProjectName       string
	ProjectVersion    string
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
func getWriteInfo(message internal.LagoonInsightsMessage, templates DependencyTrackTemplates) (dependencyTrackWriteInfo, error) {

	writeinfo := dependencyTrackWriteInfo{}

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

	// okay - now we can populate the writeinfo struct with templating

	if len(templates.ParentProjectNameTemplate) > 0 {
		n, err := processTemplate(templates.ParentProjectNameTemplate, templateValues)
		if err != nil {
			return writeinfo, err
		}
		writeinfo.ParentProjectName = n
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

func (d *DependencyTrackPostProcess) PostProcess(message internal.LagoonInsightsMessage) error {

	// first, filter the type - we're only interested in sboms
	if message.Type != internal.InsightsTypeSBOM {
		return nil
	}

	// Now we pull out the necessary information to structure the write in terms of project name and parent.
	writeInfo, err := getWriteInfo(message, d.Templates)
	if err != nil {
		return err
	}

	client, err := dtrack.NewClient(d.ApiEndpoint, dtrack.WithAPIKey(d.ApiKey))
	if err != nil {
		return err
	}

	// let's get or create the parent project
	project, err := d.getOrCreateProject(client, writeInfo.ParentProjectName)
	if err != nil {
		return err
	}

	// let's now unzip the binary payload
	var unzippedPayload bytes.Buffer

	// we need to pull the binary payload out of the message
	// get the first item from map
	var messageBinaryPayload []byte
	for _, v := range message.BinaryPayload {
		messageBinaryPayload = v
		break
	}

	err = unzipByteStream(bytes.NewReader(messageBinaryPayload), &unzippedPayload)

	if err != nil {
		return err
	}

	request := dtrack.BOMUploadRequest{
		ProjectName:    writeInfo.ProjectName,
		ParentUUID:     &project.UUID,
		AutoCreate:     true,
		ProjectVersion: writeInfo.ProjectVersion,
		BOM:            base64.StdEncoding.EncodeToString(unzippedPayload.Bytes()), //base64.StdEncoding.EncodeToString(unzippedPayload.Bytes()),
	}

	uploadToken, err := client.BOM.Upload(context.TODO(), request)
	if err != nil {
		return err
	}

	const tickerHeartbeatDuration = 1 * time.Second
	const tickerTimeout = 300 * time.Second

	var (
		doneChan = make(chan struct{})
		errChan  = make(chan error)
		ticker   = time.NewTicker(tickerHeartbeatDuration)
		timeout  = time.After(tickerTimeout)
	)

	go func() {
		defer func() {
			close(doneChan)
			close(errChan)
		}()

		for {
			select {
			case <-ticker.C:
				processing, err := client.Event.IsBeingProcessed(context.TODO(), dtrack.EventToken(uploadToken))
				if err != nil {
					errChan <- err
					return
				}
				if !processing {
					doneChan <- struct{}{}
					return
				}
			case <-timeout:
				errChan <- fmt.Errorf("timeout exceeded")
				return
			}
		}
	}()

	select {
	case <-doneChan:
		return nil
	case err = <-errChan:
		return err
	}
}

// This will get or create a project in DependencyTrack
func (d *DependencyTrackPostProcess) getOrCreateProject(client *dtrack.Client, projectName string) (dtrack.Project, error) {
	// let's ensure we have a parent project
	var project dtrack.Project
	projects, err := client.Project.GetProjectsForName(context.TODO(), projectName, true, true)
	if err != nil {
		return dtrack.Project{}, err
	}

	// let's create the parent project if it doesn't exist
	if len(projects) == 0 {
		project, err = client.Project.Create(context.TODO(), dtrack.Project{
			Name:   projectName,
			Active: true,
		})

		if err != nil {
			return dtrack.Project{}, err
		}
	} else {
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
