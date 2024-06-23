package controllers

import (
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"reflect"
	"testing"
)

func Test_generateScanPodSpec(t *testing.T) {
	type args struct {
		images    []string
		buildName string
		namespace string
	}
	tests := []struct {
		name    string
		args    args
		want    *corev1.Pod
		wantErr bool
	}{
		{
			name:    "No images",
			args:    args{images: nil, namespace: "testns", buildName: "buildnamehere"},
			wantErr: true,
		},
		{
			name: "FoundImages",
			args: args{
				images:    []string{"image1", "image2"},
				namespace: "testns",
				buildName: "buildnamehere",
			},
			want: &corev1.Pod{
				ObjectMeta: v1.ObjectMeta{
					Namespace: "testns",
					Name:      scannerNameFromBuildname("buildnamehere"),
					Labels:    imageScanPodLabels(),
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: "lagoon-deployer",
					Containers: []corev1.Container{
						{
							Name:  "scanner",
							Image: scanImageName,
							Env: []corev1.EnvVar{
								{
									Name:  "INSIGHT_SCAN_IMAGES",
									Value: "image1,image2",
								},
								{
									Name:  "NAMESPACE",
									Value: "testns",
								},
							},
							ImagePullPolicy: "Always",
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "lagoon-internal-registry-secret",
									MountPath: "/home/.docker/",
									ReadOnly:  true,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "lagoon-internal-registry-secret",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: "lagoon-internal-registry-secret",
									Items: []corev1.KeyToPath{
										{Key: ".dockerconfigjson", Path: "config.json"},
									},
								},
							},
						},
					},
					RestartPolicy: "Never",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := generateScanPodSpec(tt.args.images, tt.args.buildName, tt.args.namespace)
			if (err != nil) != tt.wantErr {
				t.Errorf("generateScanPodSpec() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("generateScanPodSpec() got = %v, want %v", got, tt.want)
			}
		})
	}
}
