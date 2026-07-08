package controllers

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_generateScanPodSpec(t *testing.T) {
	type args struct {
		images       []string
		buildPod     *corev1.Pod
		extraEnvVars map[string]string
	}
	tests := []struct {
		name    string
		args    args
		want    *corev1.Pod
		wantErr bool
	}{
		{
			name: "No images to scan",
			args: args{
				images:       nil,
				buildPod:     &corev1.Pod{},
				extraEnvVars: nil,
			},
			wantErr: true,
		},
		{
			name: "Basic images to scan",
			args: args{
				images: []string{"image1", "image2"},
				buildPod: &corev1.Pod{
					ObjectMeta: v1.ObjectMeta{
						Namespace: "testns",
						Name:      "buildnamehere",
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Env: []corev1.EnvVar{
									{
										Name:  "PROJECT",
										Value: "projectName",
									},
									{
										Name:  "ENVIRONMENT",
										Value: "environmentName",
									},
								},
							},
						},
					},
				},
				extraEnvVars: nil,
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
							Image: "scanImageName",
						Env: []corev1.EnvVar{
							{Name: "DOCKER_HOST", Value: defaultDockerhost},
							{Name: "ENVIRONMENT", Value: "environmentName"},
							{Name: "INSIGHT_SCAN_IMAGES", Value: "image1,image2"},
							{Name: "NAMESPACE", Value: "testns"},
							{Name: "PROJECT", Value: "projectName"},
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
	{
		name: "Copy build pod env vars",
			args: args{
				images: []string{"image1", "image2"},
				buildPod: &corev1.Pod{
					ObjectMeta: v1.ObjectMeta{
						Namespace: "testns",
						Name:      "buildnamehere",
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Env: []corev1.EnvVar{
									{
										Name:  "PROJECT",
										Value: "projectName",
									},
									{
										Name:  "ENVIRONMENT",
										Value: "environmentName",
									},
									{
										Name:  "ENVIRONMENT_ID",
										Value: "10",
									},
									{
										Name:  "LAGOON_ENVIRONMENT_VARIABLES",
										Value: "lagoonEnvironmentVariables",
									},
									{
										Name:  "LAGOON_FEATURE_FLAG_TEST",
										Value: "lagoonFeatureFlagTest",
									},
									{
										Name:  "LAGOON_FEATURE_FLAG_INSIGHTS",
										Value: "lagoonFeatureFlagInsights",
									},
									{
										Name:  "LAGOON_FEATURE_FLAG_",
										Value: "lagoonFeatureFlag",
									},
								},
							},
						},
					},
				},
				extraEnvVars: nil,
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
							Image: "scanImageName",
						Env: []corev1.EnvVar{
							{Name: "DOCKER_HOST", Value: defaultDockerhost},
							{Name: "ENVIRONMENT", Value: "environmentName"},
							{Name: "INSIGHT_SCAN_IMAGES", Value: "image1,image2"},
							{Name: "LAGOON_ENVIRONMENT_VARIABLES", Value: "lagoonEnvironmentVariables"},
							{Name: "LAGOON_FEATURE_FLAG_INSIGHTS", Value: "lagoonFeatureFlagInsights"},
							{Name: "LAGOON_FEATURE_FLAG_TEST", Value: "lagoonFeatureFlagTest"},
							{Name: "NAMESPACE", Value: "testns"},
							{Name: "PROJECT", Value: "projectName"},
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
		{
			name: "Extra env vars are added to scan pod",
			args: args{
				images: []string{"image1"},
				buildPod: &corev1.Pod{
					ObjectMeta: v1.ObjectMeta{
						Namespace: "testns",
						Name:      "buildnamehere",
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Env: []corev1.EnvVar{
									{
										Name:  "PROJECT",
										Value: "projectName",
									},
								},
							},
						},
					},
				},
				extraEnvVars: map[string]string{
					"SYFT_PARALLELISM": "2",
					"SYFT_LOG_LEVEL":   "warn",
				},
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
							Image: "scanImageName",
						Env: []corev1.EnvVar{
							{Name: "DOCKER_HOST", Value: defaultDockerhost},
							{Name: "INSIGHT_SCAN_IMAGES", Value: "image1"},
							{Name: "NAMESPACE", Value: "testns"},
							{Name: "PROJECT", Value: "projectName"},
							{Name: "SYFT_LOG_LEVEL", Value: "warn"},
							{Name: "SYFT_PARALLELISM", Value: "2"},
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
			got, err := generateScanPodSpec(tt.args.buildPod, tt.args.images, "scanImageName", defaultDockerhost, tt.args.extraEnvVars)
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

func Test_extractDockerHost(t *testing.T) {
	tests := []struct {
		name              string
		buildPod          *corev1.Pod
		defaultDockerHost string
		want              string
		wantErr           bool
	}{
		{
			name: "Pod with dockerhost annotation",
			buildPod: &corev1.Pod{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-build-pod",
					Namespace: "test-ns",
					Annotations: map[string]string{
						"dockerhost.lagoon.sh/name": "custom-docker-host.svc",
					},
				},
			},
			want:    "custom-docker-host.svc",
			wantErr: false,
		},
		{
			name: "Pod without dockerhost annotation",
			buildPod: &corev1.Pod{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-build-pod",
					Namespace: "test-ns",
					Annotations: map[string]string{
						"other.annotation": "some-value",
					},
				},
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "Pod with nil annotations",
			buildPod: &corev1.Pod{
				ObjectMeta: v1.ObjectMeta{
					Name:        "test-build-pod",
					Namespace:   "test-ns",
					Annotations: nil,
				},
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "Pod with empty annotations",
			buildPod: &corev1.Pod{
				ObjectMeta: v1.ObjectMeta{
					Name:        "test-build-pod",
					Namespace:   "test-ns",
					Annotations: map[string]string{},
				},
			},
			want:    "",
			wantErr: true,
		},
		{
			name: "Pod with empty dockerhost annotation value",
			buildPod: &corev1.Pod{
				ObjectMeta: v1.ObjectMeta{
					Name:      "test-build-pod",
					Namespace: "test-ns",
					Annotations: map[string]string{
						"dockerhost.lagoon.sh/name": "",
					},
				},
			},
			want:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := extractDockerHost(tt.buildPod)
			if (err != nil) != tt.wantErr {
				t.Errorf("extractDockerHost() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("extractDockerHost() = %v, want %v", got, tt.want)
			}
		})
	}
}
