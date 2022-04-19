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
	"github.com/cheshir/go-mq"
	"github.com/robfig/cron/v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"log"
	"os"
	client2 "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"strconv"
	"time"

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
	scheme                       = runtime.NewScheme()
	setupLog                     = ctrl.Log.WithName("setup")
	mqEnable                     bool
	mqUser                       string
	mqPass                       string
	mqHost                       string
	mqPort                       string
	rabbitReconnectRetryInterval int
	burnAfterReading             bool
	clearConfigmapCronSched      string
	mqConfig                     mq.Config
)

const BROKER_PRODUCER_NAME = "lagoon-insights"

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
		//log.Error(err, "Unable to write to message broker")
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
		"Used primarily for debugging to disable the Message Broker connection.")
	flag.StringVar(&mqUser, "rabbitmq-username", "guest",
		"The username of the rabbitmq user.")
	flag.StringVar(&mqPass, "rabbitmq-password", "guest",
		"The password for the rabbitmq user.")
	flag.StringVar(&mqHost, "rabbitmq-hostname", "localhost",
		"The hostname for the rabbitmq host.")
	flag.StringVar(&mqPort, "rabbitmq-port", "5672",
		"The port for the rabbitmq host.")
	flag.IntVar(&rabbitReconnectRetryInterval, "rabbitmq-reconnect-retry-interval", 30,
		"The retry interval for rabbitmq.")
	flag.BoolVar(&burnAfterReading, "burn-after-reading", false,
		"Remove insights configmaps after they have been processed.")
	flag.StringVar(&clearConfigmapCronSched, "clear-configmap-sched", "* * * * *",
		"The cron schedule specifying how often insightType configmaps should be cleared.")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	//Grab overrides from environment where appropriate
	mqUser = getEnv("RABBITMQ_USERNAME", mqUser)
	mqPass = getEnv("RABBITMQ_PASSWORD", mqPass)
	mqHost = getEnv("RABBITMQ_ADDRESS", mqHost)
	mqPort = getEnv("RABBITMQ_PORT", mqPort)

	//Check burn after reading value from environment
	if getEnv("BURN_AFTER_READING", "FALSE") == "TRUE" {
		log.Printf("Burn-after-reading enabled via environment variable")
		burnAfterReading = true
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
		DSN: fmt.Sprintf("amqp://%s:%s@%s/", mqUser, mqPass, mqHost),
	}

	//messageQ, err := mq.New(mqConfig)
	//if err != nil {
	//	log.Fatalf("Failed to set handler to consumer `%s`: %v", "consumer_name", err)
	//	os.Exit(1)
	//}

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "ac8a682b.lagoon.sh",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&controllers.ConfigMapReconciler{
		Client:           mgr.GetClient(),
		Scheme:           mgr.GetScheme(),
		MessageQWriter:   mqWriteObject,
		BurnAfterReading: burnAfterReading,
		WriteToQueue:     mqEnable,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ConfigMap")
		os.Exit(1)
	}

	// Set up periodic removal of processed configmaps
	if burnAfterReading {
		startBurnAfterReadingCron(mgr)
	}

	startInsightsDeferredClearCron(mgr)

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
		insightsProcessedRequirement, err := labels.NewRequirement(controllers.InsightsLabel, selection.Exists, []string{})
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
			if _, okay := x.Annotations[controllers.InsightsUpdatedAnnotationLabel]; okay {
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
		insightsDeferredRequirement, err := labels.NewRequirement(controllers.InsightsWriteDeferred, selection.Exists, []string{})
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
			if writeDeferredVal, okay := x.Labels[controllers.InsightsWriteDeferred]; okay {
				writeDeferredValTS, _ := strconv.ParseInt(writeDeferredVal, 10, 32)
				parsed := time.Unix(writeDeferredValTS, 0)
				if err != nil {
					log.Printf("Unable to parse string '%v' for '%v' in ns '%v': %v\n\n", writeDeferredVal, x.Name, x.Namespace, err)
					continue
				}

				if time.Now().After(parsed) {
					delete(x.Labels, controllers.InsightsWriteDeferred)
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

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

// accepts fallback values 1, t, T, TRUE, true, True, 0, f, F, FALSE, false, False
// anything else is false.
func getEnvBool(key string, fallback bool) bool {
	if value, ok := os.LookupEnv(key); ok {
		rVal, _ := strconv.ParseBool(value)
		return rVal
	}
	return fallback
}
