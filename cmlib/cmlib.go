package cmlib

import (
	"context"
	"errors"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func LabelCM(ctx context.Context, r client.Client, configMap corev1.ConfigMap, labelKey string, labelValue string) error {
	return BatchUpdateCM(ctx, r, configMap, map[string]string{
		labelKey: labelValue,
	}, nil)
}

func AnnotateCM(ctx context.Context, r client.Client, configMap corev1.ConfigMap, annotationKey string, annotationValue string) error {
	return BatchUpdateCM(ctx, r, configMap, nil, map[string]string{
		annotationKey: annotationValue,
	})
}

func BatchUpdateCM(ctx context.Context, r client.Client, configMap corev1.ConfigMap, updatedLabels map[string]string, updatedAnnotations map[string]string) error {
	log := log.FromContext(ctx)
	if len(updatedLabels) == 0 && len(updatedAnnotations) == 0 {
		return errors.New("no updates passed to BatchUpdateCM")
	}

	updatedItems := []string{}

	if len(updatedLabels) > 0 {
		labels := configMap.GetLabels()
		if labels == nil {
			labels = map[string]string{}
		}

		for labelKey, labelValue := range updatedLabels {
			labels[labelKey] = labelValue
		}

		configMap.SetLabels(labels)
		updatedItems = append(updatedItems, "Labels")
	}

	if len(updatedAnnotations) > 0 {
		annotations := configMap.GetAnnotations()
		if annotations == nil {
			annotations = map[string]string{}
		}

		for key, value := range updatedAnnotations {
			annotations[key] = value
		}

		configMap.SetAnnotations(annotations)
		updatedItems = append(updatedItems, "Annotations")
	}

	if err := r.Update(ctx, &configMap); err != nil {
		log.Error(err, fmt.Sprintf("unable to update configMap - setting: %v", strings.Join(updatedItems, ",")))
		return err
	}
	return nil
}
