package cmlib

import (
	"context"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func LabelCM(ctx context.Context, r client.Client, configMap corev1.ConfigMap, labelKey string, labelValue string) error {
	log := log.FromContext(ctx)
	labels := configMap.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}

	labels[labelKey] = labelValue
	configMap.SetLabels(labels)

	if err := r.Update(ctx, &configMap); err != nil {
		log.Error(err, "Unable to update configMap - setting labels")
		return err
	}
	return nil
}

func AnnotateCM(ctx context.Context, r client.Client, configMap corev1.ConfigMap, annotationKey string, annotationValue string) error {
	log := log.FromContext(ctx)
	annotations := configMap.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}

	annotations[annotationKey] = annotationValue
	configMap.SetAnnotations(annotations)

	if err := r.Update(ctx, &configMap); err != nil {
		log.Error(err, "Unable to update configMap - setting annotations")
		return err
	}
	return nil
}
