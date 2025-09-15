package deptrack

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"time"

	dtrack "github.com/DependencyTrack/client-go"
	"lagoon.sh/insights-remote/internal"
)

// DefaultPostProcess will send insights for all namespaces to a central Dependency Track instance.
type DefaultPostProcess struct {
	ApiEndpoint string
	ApiKey      string
	Templates   Templates
}

func (d *DefaultPostProcess) PostProcess(message internal.LagoonInsightsMessage) error {

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

	// Here we iterate over the parent projects, creating them if/when we need
	// only the last one gets passed to the upload
	var project *dtrack.Project
	for _, projectName := range writeInfo.ParentProjectNames {
		var parentRef *dtrack.ParentRef
		if project != nil {
			parentRef = &dtrack.ParentRef{UUID: project.UUID}
		}
		projectObj, err := getOrCreateProject(client, projectName, parentRef)
		if err != nil {
			return err
		}
		project = &projectObj
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
			ticker.Stop()
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
