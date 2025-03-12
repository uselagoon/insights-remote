/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"flag"
	"fmt"
	"lagoon.sh/insights-remote/internal"
	"lagoon.sh/insights-remote/internal/postprocess"
	"log"
	"os"
	"strconv"
	"time"

	"lagoon.sh/insights-remote/internal/service"
	"lagoon.sh/insights-remote/internal/tokens"

	"github.com/cheshir/go-mq/v2"
	"github.com/robfig/cron/v3"
	"github.com/uselagoon/machinery/utils/variables"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	client2 "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"lagoon.sh/insights-remote/controllers"
	//+kubebuilder:scaffold:imports
)

var (
	scheme                                   = runtime.NewScheme()
	setupLog                                 = ctrl.Log.WithName("setup")
	mqEnable                                 bool
	mqUser                                   string
	mqPass                                   string
	mqHost                                   string
	mqTLS                                    bool
	mqVerify                                 bool
	mqCACert                                 string
	mqClientCert                             string
	mqClientKey                              string
	rabbitReconnectRetryInterval             int
	burnAfterReading                         bool
	clearConfigmapCronSched                  string
	mqConfig                                 mq.Config
	insightsTokenSecret                      string
	enableNSReconciler                       bool
	enableCMReconciler                       bool
	enableInsightDeferred                    bool //TODO: Better names for this
	enableWebservice                         bool
	tokenTargetLabel                         string
	webservicePort                           string
	generateTokenOnly                        bool
	generateTokenOnlyNamespace               string
	generateTokenOnlyEnvironmentId           string
	generateTokenOnlyProjectName             string
	generateTokenOnlyEnvironmentName         string
	enableDependencyTrackIntegration         bool
	dependencyTrackApiEndpoint               string
	dependencyTrackApiKey                    string
	dependencyTrackProjectNameTemplate       string
	dependencyTrackParentProjectNameTemplate string
	dependencyTrackVersionTemplate           string
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	//+kubebuilder:scaffold:scheme
}

func mqWriteObject(data []byte) error {
	messageQ, err := mq.New(mqConfig)
	if err != nil {
		//TODO: Log useful data here ...
		return err
	}
	defer messageQ.Close()

	producer, err := messageQ.SyncProducer("lagoon-insights")
	if err != nil {
		//log.Error(err, "Unable to write to message broker")
		return err
	}

	err = producer.Produce(data)

	if err != nil {
		return err
	}

	return nil
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&mqEnable, "rabbitmq-enabled", true,
		"Used primarily for debugging to disable the Message Broker connection (env var: RABBITMQ_ENABLED).")
	flag.StringVar(&mqUser, "rabbitmq-username", "guest",
		"The username of the rabbitmq user (env var: RABBITMQ_USERNAME).")
	flag.StringVar(&mqPass, "rabbitmq-password", "guest",
		"The password for the rabbitmq user (env var: RABBITMQ_PASSWORD).")
	flag.StringVar(&mqHost, "rabbitmq-hostname", "localhost",
		"The hostname for the rabbitmq host (env var: RABBITMQ_ADDRESS).")
	flag.BoolVar(&mqTLS, "rabbitmq-tls", false,
		"To use amqps instead of amqp.")
	flag.BoolVar(&mqVerify, "rabbitmq-verify", false,
		"To verify rabbitmq peer connection.")
	flag.StringVar(&mqCACert, "rabbitmq-cacert", "",
		"The path to the ca certificate")
	flag.StringVar(&mqClientCert, "rabbitmq-clientcert", "",
		"The path to the client certificate")
	flag.StringVar(&mqClientKey, "rabbitmq-clientkey", "",
		"The path to the client key")
	flag.IntVar(&rabbitReconnectRetryInterval, "rabbitmq-reconnect-retry-interval", 30,
		"The retry interval for rabbitmq.")
	flag.BoolVar(&burnAfterReading, "burn-after-reading", false,
		"Remove insights configmaps after they have been processed (env var: BURN_AFTER_READING).")
	flag.StringVar(&clearConfigmapCronSched, "clear-configmap-sched", "* * * * *",
		"The cron schedule specifying how often insightType configmaps should be cleared (env var: CLEAR_CONFIGMAP_SCHED).")

	flag.StringVar(&insightsTokenSecret, "insights-token-secret", "testsecret",
		"The secret used to create the insights tokens used to communicate back to the webservice (env var: INSIGHTS_TOKEN_SECRET).")

	flag.BoolVar(&enableCMReconciler, "enable-configmap-reconciler", true,
		"Enable the configmap reconciler (env var: ENABLE_CONFIGMAP_RECONCILER).")

	flag.BoolVar(&enableNSReconciler, "enable-namespace-reconciler", true,
		"enable-namespace-reconciler (env var: ENABLE_NAMESPACE_RECONCILER).")

	flag.BoolVar(&enableInsightDeferred, "enable-insights-deferred", false,
		"Delete insights after certain time (env var: ENABLE_INSIGHTS_DEFERRED).")

	flag.BoolVar(&enableWebservice, "enable-webservice", true,
		"Enables json endpoint for writing insights data (env var: ENABLE_WEBSERVICE).")

	flag.StringVar(&tokenTargetLabel, "token-target-label", "",
		"Constrain webservice token generation to namespaces with this label (env var: TOKEN_TARGET_LABEL).")

	flag.StringVar(&webservicePort, "webservice-port", "8888",
		"Port on which we expose the JSON webservice (env var: WEBSERVICE_PORT).")

	flag.BoolVar(&generateTokenOnly, "generate-token-only", false, "Generate a token and exit.")

	flag.StringVar(&generateTokenOnlyNamespace, "generate-token-only-namespace", "", "Namespace for which to generate a token.")

	flag.StringVar(&generateTokenOnlyEnvironmentId, "generate-token-only-environment-id", "", "EnvironmentName ID for which to generate a token.")

	flag.StringVar(&generateTokenOnlyProjectName, "generate-token-only-project-name", "", "ProjectName name for which to generate a token.")

	flag.StringVar(&generateTokenOnlyEnvironmentName, "generate-token-only-environment-name", "", "EnvironmentName name for which to generate a token.")

	// If these two are set, we will post to Dependency Track post-process
	flag.BoolVar(&enableDependencyTrackIntegration, "enable-dependency-track-integration", false, "Enable Dependency Track integration.")
	flag.StringVar(&dependencyTrackApiEndpoint, "dependency-track-api-endpoint", "", "The endpoint for the Dependency Track API.")
	flag.StringVar(&dependencyTrackApiKey, "dependency-track-api-key", "", "The API key for the Dependency Track API.")
	flag.StringVar(&dependencyTrackProjectNameTemplate, "dependency-track-project-name-template", "{{ .ProjectName }}-{{ .EnvironmentName }}", "The template for the project name in Dependency Track.")
	flag.StringVar(&dependencyTrackParentProjectNameTemplate, "dependency-track-parent-project-name-template", "{{ .ProjectName }}", "The template for the parent project name in Dependency Track.")
	flag.StringVar(&dependencyTrackVersionTemplate, "dependency-track-version-template", "unset", "The template for the version in Dependency Track.")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	//Generate a token and exit if generateTokenOnly is set
	if generateTokenOnly {
		if generateTokenOnlyEnvironmentName == "" || generateTokenOnlyEnvironmentId == "" || generateTokenOnlyProjectName == "" || generateTokenOnlyNamespace == "" {
			log.Fatal("generate-token-only requires all of generate-token-only-environment-name, generate-token-only-environment-id, generate-token-only-project-name and generate-token-only-namespace to be set")
			os.Exit(1)
		}
		jwt, err := tokens.GenerateTokenForNamespace(insightsTokenSecret, tokens.NamespaceDetails{
			Namespace:       generateTokenOnlyNamespace,
			EnvironmentId:   generateTokenOnlyEnvironmentId,
			ProjectName:     generateTokenOnlyProjectName,
			EnvironmentName: generateTokenOnlyEnvironmentName,
		})
		if err != nil {
			log.Fatal(err, "Unable to generate token")
		}
		fmt.Println(jwt)
		os.Exit(0)
	}

	//Grab overrides from environment where appropriate

	mqUser = variables.GetEnv("RABBITMQ_USERNAME", mqUser)
	mqPass = variables.GetEnv("RABBITMQ_PASSWORD", mqPass)
	mqHost = variables.GetEnv("RABBITMQ_ADDRESS", mqHost)
	//rabbitReconnectRetryInterval = variables.GetEnv("RABBITMQ_RECONNECT_RETRY_INTERVAL", rabbitReconnectRetryInterval)
	mqEnable = variables.GetEnvBool("RABBITMQ_ENABLED", mqEnable)
	mqTLS = variables.GetEnvBool("RABBITMQ_TLS", mqTLS)
	mqCACert = variables.GetEnv("RABBITMQ_CACERT", mqCACert)
	mqClientCert = variables.GetEnv("RABBITMQ_CLIENTCERT", mqClientCert)
	mqClientKey = variables.GetEnv("RABBITMQ_CLIENTKEY", mqClientKey)
	mqVerify = variables.GetEnvBool("RABBITMQ_VERIFY", mqVerify)

	insightsTokenSecret = variables.GetEnv("INSIGHTS_TOKEN_SECRET", insightsTokenSecret)
	clearConfigmapCronSched = variables.GetEnv("CLEAR_CONFIGMAP_SCHED", clearConfigmapCronSched)
	enableCMReconciler = variables.GetEnvBool("ENABLE_CONFIGMAP_RECONCILER", enableNSReconciler)
	enableInsightDeferred = variables.GetEnvBool("ENABLE_INSIGHTS_DEFERRED", enableInsightDeferred)
	enableNSReconciler = variables.GetEnvBool("ENABLE_NAMESPACE_RECONCILER", enableNSReconciler)
	enableWebservice = variables.GetEnvBool("ENABLE_WEBSERVICE", enableWebservice)
	tokenTargetLabel = variables.GetEnv("TOKEN_TARGET_LABEL", tokenTargetLabel)
	webservicePort = variables.GetEnv("WEBSERVICE_PORT", webservicePort)
	//Check burn after reading value from environment
	if variables.GetEnv("BURN_AFTER_READING", "FALSE") == "TRUE" {
		log.Printf("Burn-after-reading enabled via environment variable")
		burnAfterReading = true
	}
	// Check if Dependency Track integration is enabled
	enableDependencyTrackIntegration = variables.GetEnvBool("ENABLE_DEPENDENCY_TRACK_INTEGRATION", enableDependencyTrackIntegration)
	dependencyTrackApiEndpoint = variables.GetEnv("DEPENDENCY_TRACK_API_ENDPOINT", dependencyTrackApiEndpoint)
	dependencyTrackApiKey = variables.GetEnv("DEPENDENCY_TRACK_API_KEY", dependencyTrackApiKey)
	// Check if Dependency Track integration is enabled - fail if missing required configuration
	if enableDependencyTrackIntegration && (dependencyTrackApiEndpoint == "" || dependencyTrackApiKey == "") {
		log.Fatal("Dependency Track integration enabled but missing required configuration")
	}

	brokerDSN := fmt.Sprintf("amqp://%s:%s@%s", mqUser, mqPass, mqHost)
	if mqTLS {
		verify := "verify_none"
		if mqVerify {
			verify = "verify_peer"
		}
		brokerDSN = fmt.Sprintf("amqps://%s:%s@%s?verify=%s", mqUser, mqPass, mqHost, verify)
		if mqCACert != "" {
			brokerDSN = fmt.Sprintf("%s&cacertfile=%s", brokerDSN, mqCACert)
		}
		if mqClientCert != "" {
			brokerDSN = fmt.Sprintf("%s&certfile=%s", brokerDSN, mqClientCert)
		}
		if mqClientKey != "" {
			brokerDSN = fmt.Sprintf("%s&keyfile=%s", brokerDSN, mqClientKey)
		}
	}
	mqConfig = mq.Config{
		ReconnectDelay: time.Duration(rabbitReconnectRetryInterval) * time.Second,
		Exchanges: mq.Exchanges{
			{
				Name: "lagoon-insights",
				Type: "direct",
				Options: mq.Options{
					"durable":       true,
					"delivery_mode": "2",
					"headers":       "",
					"content_type":  "",
				},
			},
		},
		Queues: mq.Queues{
			{
				Name:     "lagoon-insights:items",
				Exchange: "lagoon-insights",
				Options: mq.Options{
					"durable":       true,
					"delivery_mode": "2",
					"headers":       "",
					"content_type":  "",
				},
			},
		},
		Producers: mq.Producers{
			{
				Name:     "lagoon-insights",
				Exchange: "lagoon-insights",
				Sync:     true,
				Options: mq.Options{
					"delivery_mode": "2",
					"headers":       "",
					"content_type":  "",
				},
			},
		},
		DSN: brokerDSN,
	}

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                server.Options{BindAddress: metricsAddr},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "ac8a682b.lagoon.sh",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if enableCMReconciler {

		// First, let's set up post processors
		postProcessor := postprocess.PostProcessors{}

		if enableDependencyTrackIntegration && dependencyTrackApiEndpoint != "" && dependencyTrackApiKey != "" {
			postProcessor.PostProcessors = append(postProcessor.PostProcessors, &postprocess.DependencyTrackPostProcess{
				ApiEndpoint: dependencyTrackApiEndpoint,
				ApiKey:      dependencyTrackApiKey,
				Templates: postprocess.DependencyTrackTemplates{
					ParentProjectNameTemplate: dependencyTrackParentProjectNameTemplate,
					ProjectNameTemplate:       dependencyTrackProjectNameTemplate,
					VersionTemplate:           dependencyTrackVersionTemplate,
				},
			})
		}

		// Set up the controller
		if err = (&controllers.ConfigMapReconciler{
			Client:           mgr.GetClient(),
			Scheme:           mgr.GetScheme(),
			MessageQWriter:   mqWriteObject,
			BurnAfterReading: burnAfterReading,
			WriteToQueue:     mqEnable,
			PostProcessors:   postProcessor,
		}).SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "ConfigMap")
			os.Exit(1)
		}
	} else {
		log.Printf("CM Reconciler disabled - skipping")
	}

	// Set up periodic removal of processed configmaps
	if burnAfterReading {
		startBurnAfterReadingCron(mgr)
	} else {
		log.Printf("Burn after reading disabled - skipping")
	}

	if enableInsightDeferred {
		startInsightsDeferredClearCron(mgr)
	} else {
		log.Printf("Insights deferred disabled - skipping")
	}

	if enableNSReconciler {
		if err = (&controllers.NamespaceReconciler{
			Client:            mgr.GetClient(),
			Scheme:            mgr.GetScheme(),
			InsightsJWTSecret: insightsTokenSecret,
		}).SetupWithManager(mgr, tokenTargetLabel); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "Namespace")
			os.Exit(1)
		}
	} else {
		log.Printf("Namespace reconciler disabled - skipping")
	}

	if enableWebservice {
		log.Println("Enabling JSON endpoint ...")
		startInsightsEndpoint(mgr)
	} else {
		log.Printf("Namespace reconciler disabled - skipping")
	}

	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func startBurnAfterReadingCron(mgr manager.Manager) {
	c := cron.New()
	c.AddFunc(clearConfigmapCronSched, func() {
		client := mgr.GetClient()
		configMapList := &corev1.ConfigMapList{}
		insightsProcessedRequirement, err := labels.NewRequirement(internal.InsightsLabel, selection.Exists, []string{})
		if err != nil {
			fmt.Printf("bad requirement: %v\n\n", err)
			return
		}

		insightsProcessLabelSelector := labels.NewSelector()
		insightsProcessLabelSelector = insightsProcessLabelSelector.Add(*insightsProcessedRequirement)
		configMapListOptionSearch := client2.ListOptions{
			LabelSelector: insightsProcessLabelSelector,
			Limit:         5,
		}
		err = client.List(context.Background(), configMapList, &configMapListOptionSearch)
		if err != nil {
			log.Printf("Error getting list of configMaps: %v\n\n", err)
			return
		}

		for _, x := range configMapList.Items {
			//check the annotations
			if _, okay := x.Annotations[internal.InsightsUpdatedAnnotationLabel]; okay {
				//grab the build this is linked to
				buildName := ""
				if val, ok := x.Labels["lagoon.sh/buildName"]; ok {
					buildName = fmt.Sprintf(" (build: '%v')", val)
				}
				if err := client.Delete(context.Background(), &x); err != nil {
					log.Printf("Unable to delete configMap '%v' in ns '%v': %v\n\n", x.Name, x.Namespace, err)
				} else {
					log.Printf("Deleted Insights configMap '%v' in ns '%v' %v", x.Name, x.Namespace, buildName)
				}
			}
		}
	})
	c.Start()
}

func startInsightsDeferredClearCron(mgr manager.Manager) {
	c := cron.New()
	c.AddFunc(clearConfigmapCronSched, func() {

		client := mgr.GetClient()
		configMapList := &corev1.ConfigMapList{}
		insightsDeferredRequirement, err := labels.NewRequirement(internal.InsightsWriteDeferred, selection.Exists, []string{})
		if err != nil {
			fmt.Printf("bad requirement: %v\n\n", err)
			return
		}

		insightsDeferredLabelSelector := labels.NewSelector()
		insightsDeferredLabelSelector = insightsDeferredLabelSelector.Add(*insightsDeferredRequirement)
		configMapListOptionSearch := client2.ListOptions{
			LabelSelector: insightsDeferredLabelSelector,
			Limit:         5,
		}
		err = client.List(context.Background(), configMapList, &configMapListOptionSearch)
		if err != nil {
			log.Printf("Error getting list of configMaps: %v\n\n", err)
			return
		}

		for _, x := range configMapList.Items {
			//check the labels
			if writeDeferredVal, okay := x.Labels[internal.InsightsWriteDeferred]; okay {
				writeDeferredValTS, _ := strconv.ParseInt(writeDeferredVal, 10, 32)
				parsed := time.Unix(writeDeferredValTS, 0)
				if err != nil {
					log.Printf("Unable to parse string '%v' for '%v' in ns '%v': %v\n\n", writeDeferredVal, x.Name, x.Namespace, err)
					continue
				}

				if time.Now().After(parsed) {
					delete(x.Labels, internal.InsightsWriteDeferred)
					err = client.Update(context.Background(), &x)
					if err != nil {
						log.Printf("Unable to update configmap '%v' for '%v' in ns '%v': %v\n\n", x.Name, x.Name, x.Namespace, err)
						continue
					}

					log.Printf("Removed write deferred date '%v' for '%v' in ns '%v'\n\n", writeDeferredVal, x.Name, x.Namespace)
				}
			}
		}
	})
	c.Start()
}

func startInsightsEndpoint(mgr manager.Manager) {
	router := service.SetupRouter(insightsTokenSecret, mqWriteObject, mqEnable)
	go router.Run(fmt.Sprintf(":%v", webservicePort))
}
