package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strconv"

	"go.opencensus.io/exporter/jaeger"
	"go.opencensus.io/trace"

	docopt "github.com/docopt/docopt-go"

	"github.com/fission/fission"
	"github.com/fission/fission/buildermgr"
	"github.com/fission/fission/controller"
	"github.com/fission/fission/executor"
	"github.com/fission/fission/kubewatcher"
	"github.com/fission/fission/mqtrigger"
	"github.com/fission/fission/router"
	"github.com/fission/fission/storagesvc"
	"github.com/fission/fission/timer"
)

func runController(port int) {
	controller.Start(port, false)
	log.Fatalf("Error: Controller exited.")
}

func runRouter(port int, executorUrl string) {
	router.Start(port, executorUrl)
	log.Fatalf("Error: Router exited.")
}

func runExecutor(port int, fissionNamespace, functionNamespace, envBuilderNamespace string) {
	err := executor.StartExecutor(fissionNamespace, functionNamespace, envBuilderNamespace, port)
	if err != nil {
		log.Fatalf("Error starting executor: %v", err)
	}
}

func runKubeWatcher(routerUrl string) {
	err := kubewatcher.Start(routerUrl)
	if err != nil {
		log.Fatalf("Error starting kubewatcher: %v", err)
	}
}

func runTimer(routerUrl string) {
	err := timer.Start(routerUrl)
	if err != nil {
		log.Fatalf("Error starting timer: %v", err)
	}
}

func runMessageQueueMgr(routerUrl string) {
	err := messagequeue.Start(routerUrl)
	if err != nil {
		log.Fatalf("Error starting timer: %v", err)
	}
}

func runStorageSvc(port int, readWriteConfig map[string]string, readOnlyConfigs []map[string]string) {
	enableArchivePruner := true

	err := storagesvc.RunStorageService(port, enableArchivePruner, readWriteConfig, readOnlyConfigs)
	if err != nil {
		log.Fatalf("Error starting storage service: %v", err)
	}
}

func runBuilderMgr(storageSvcUrl string, envBuilderNamespace string) {
	err := buildermgr.Start(storageSvcUrl, envBuilderNamespace)
	if err != nil {
		log.Fatalf("Error starting buildermgr: %v", err)
	}
}

func getPort(portArg interface{}) int {
	portArgStr := portArg.(string)
	port, err := strconv.Atoi(portArgStr)
	if err != nil {
		log.Fatalf("Error: invalid port number '%v'", portArgStr)
	}
	return port
}

func getStringArgWithDefault(arg interface{}, defaultValue string) string {
	if arg != nil {
		return arg.(string)
	} else {
		return defaultValue
	}
}

func registerTraceExporter(arguments map[string]interface{}) error {
	collectorEndpoint := getStringArgWithDefault(arguments["--jaegerCollectorEndpoint"], "")
	if collectorEndpoint == "" {
		log.Print("Skipping trace exporter registration")
		return nil
	}

	serviceName := "Fission-Unknown"

	if arguments["--controllerPort"] != nil {
		serviceName = "Fission-Controller"
	} else if arguments["--routerPort"] != nil {
		serviceName = "Fission-Router"
	} else if arguments["--executorPort"] != nil {
		serviceName = "Fission-Executor"
	} else if arguments["--kubewatcher"] == true {
		serviceName = "Fission-KubeWatcher"
	} else if arguments["--timer"] == true {
		serviceName = "Fission-Timer"
	} else if arguments["--mqt"] == true {
		serviceName = "Fission-MessageQueueMgr"
	} else if arguments["--builderMgr"] == true {
		serviceName = "Fission-BuilderMgr"
	} else if arguments["--storageServicePort"] != nil {
		serviceName = "Fission-StorageSvc"
	}

	exporter, err := jaeger.NewExporter(jaeger.Options{
		CollectorEndpoint: collectorEndpoint,
		ServiceName:       serviceName,
		Process: jaeger.Process{
			ServiceName: serviceName,
			Tags: []jaeger.Tag{
				// jaeger.StringTag("ip", "127.0.0.1"),
				jaeger.BoolTag("fission", true),
			},
		},
	})
	if err != nil {
		return err
	}
	trace.RegisterExporter(exporter)
	trace.ApplyConfig(trace.Config{DefaultSampler: trace.AlwaysSample()})
	return nil
}

func main() {
	// From https://github.com/containous/traefik/pull/1817/files
	// Tell glog to log into STDERR. Otherwise, we risk
	// certain kinds of API errors getting logged into a directory not
	// available in a `FROM scratch` Docker container, causing glog to abort
	// hard with an exit code > 0.
	flag.Set("logtostderr", "true")

	usage := `fission-bundle: Package of all fission microservices: controller, router, executor.

Use it to start one or more of the fission servers:

 Controller is a stateless API frontend for fission resources.

 Pool manager maintains a pool of generalized function containers, and
 specializes them on-demand. Executor must be run from a pod in a
 Kubernetes cluster.

 Router implements HTTP triggers: it routes to running instances,
 working with the controller and executor.

 Kubewatcher implements Kubernetes Watch triggers: it watches
 Kubernetes resources and invokes functions described in the
 KubernetesWatchTrigger.

 The storage service implements storage for functions too large to fit
 in the Kubernetes API resource object. It supports various storage
 backends.

Usage:
  fission-bundle --controllerPort=<port> [--jaegerCollectorEndpoint=<url>]
  fission-bundle --routerPort=<port> [--executorUrl=<url>] [--jaegerCollectorEndpoint=<url>]
  fission-bundle --executorPort=<port> [--namespace=<namespace>] [--fission-namespace=<namespace>] [--jaegerCollectorEndpoint=<url>]
  fission-bundle --kubewatcher [--routerUrl=<url>] [--jaegerCollectorEndpoint=<url>]
  fission-bundle --storageServicePort=<port> --filePath=<filePath> [--jaegerCollectorEndpoint=<url>]
  fission-bundle --storageServicePort=<port> [--read-write=<provider>] [--local=<path> --local-subdir=<subdir>] [--gcs-bucket=<bucket> --gcs-json-file=<path> --gcs-project=<project>] [--jaegerCollectorEndpoint=<url>]
  fission-bundle --builderMgr [--storageSvcUrl=<url>] [--envbuilder-namespace=<namespace>] [--jaegerCollectorEndpoint=<url>]
  fission-bundle --timer [--routerUrl=<url>] [--jaegerCollectorEndpoint=<url>]
  fission-bundle --mqt   [--routerUrl=<url>] [--jaegerCollectorEndpoint=<url>]
  fission-bundle --version
Options:
  --jaegerCollectorEndpoint=<url> Jaeger HTTP Thrift collector URL.
  --controllerPort=<port>         Port that the controller should listen on.
  --routerPort=<port>             Port that the router should listen on.
  --executorPort=<port>           Port that the executor should listen on.
  --storageServicePort=<port>     Port that the storage service should listen on.
  --executorUrl=<url>             Executor URL. Not required if --executorPort is specified.
  --routerUrl=<url>               Router URL.
  --etcdUrl=<etcdUrl>             Etcd URL.
  --storageSvcUrl=<url>           StorageService URL.
  --filePath=<filePath>           Directory to store functions in.
  --namespace=<namespace>         Kubernetes namespace in which to run function containers. Defaults to 'fission-function'.
  --kubewatcher                   Start Kubernetes events watcher.
  --timer                         Start Timer.
  --mqt                           Start message queue trigger.
  --builderMgr                    Start builder manager.
  --version                       Print version information
`
	version := fmt.Sprintf("Fission Bundle Version: %v", fission.BuildInfo().String())
	arguments, err := docopt.Parse(usage, nil, true, version, false)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	err = registerTraceExporter(arguments)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	functionNs := getStringArgWithDefault(arguments["--namespace"], "fission-function")
	fissionNs := getStringArgWithDefault(arguments["--fission-namespace"], "fission")
	envBuilderNs := getStringArgWithDefault(arguments["--envbuilder-namespace"], "fission-builder")

	executorUrl := getStringArgWithDefault(arguments["--executorUrl"], "http://executor.fission")
	routerUrl := getStringArgWithDefault(arguments["--routerUrl"], "http://router.fission")
	storageSvcUrl := getStringArgWithDefault(arguments["--storageSvcUrl"], "http://storagesvc.fission")

	if arguments["--controllerPort"] != nil {
		port := getPort(arguments["--controllerPort"])
		runController(port)
	}

	if arguments["--routerPort"] != nil {
		port := getPort(arguments["--routerPort"])
		runRouter(port, executorUrl)
	}

	if arguments["--executorPort"] != nil {
		port := getPort(arguments["--executorPort"])
		runExecutor(port, fissionNs, functionNs, envBuilderNs)
	}

	if arguments["--kubewatcher"] == true {
		runKubeWatcher(routerUrl)
	}

	if arguments["--timer"] == true {
		runTimer(routerUrl)
	}

	if arguments["--mqt"] == true {
		runMessageQueueMgr(routerUrl)
	}

	if arguments["--builderMgr"] == true {
		runBuilderMgr(storageSvcUrl, envBuilderNs)
	}

	if arguments["--storageServicePort"] != nil {
		port := getPort(arguments["--storageServicePort"])
		var gcsConfig map[string]string
		var localConfig map[string]string

		filePath := getStringArgWithDefault(arguments["--local"], "")
		if filePath == "" {
			filePath = getStringArgWithDefault(arguments["--filePath"], "")
		}

		subdir := getStringArgWithDefault(arguments["--local-subdir"], "")
		if subdir == "" {
			subdir = os.Getenv("SUBDIR")
			if subdir == "" {
				subdir = "fission-functions"
			}
		}

		if filePath != "" {
			localConfig = map[string]string{
				storagesvc.ConfigKind:         storagesvc.ConfigKindLocal,
				storagesvc.ConfigLocalKeyPath: filePath,
				storagesvc.ConfigContainer:    subdir,
			}
		}

		gcsJSONFile := getStringArgWithDefault(arguments["--gcs-json-file"], "")
		if gcsJSONFile != "" {
			gcsJSON, err := ioutil.ReadFile(gcsJSONFile)
			if err != nil {
				log.Fatalf("Error reading GCS JSON file %s: %v", gcsJSONFile, err)
			}
			gcsConfig = map[string]string{
				storagesvc.ConfigKind: storagesvc.ConfigKindGCS,

				storagesvc.ConfigContainer:    getStringArgWithDefault(arguments["--gcs-bucket"], ""),
				storagesvc.ConfigGCSJSON:      string(gcsJSON),
				storagesvc.ConfigGCSProjectId: getStringArgWithDefault(arguments["--gcs-project"], ""),
			}
		}

		readWriteProvider := getStringArgWithDefault(arguments["--read-write"], "")
		if readWriteProvider == "" {
			readWriteProvider = "local"
		}

		var readWriteConfig map[string]string
		var readOnlyConfigs []map[string]string

		switch readWriteProvider {
		case "local":
			readWriteConfig = localConfig
			readOnlyConfigs = append(readOnlyConfigs, gcsConfig)
		case "gcs":
			readWriteConfig = gcsConfig
			readOnlyConfigs = append(readOnlyConfigs, localConfig)
		default:
			log.Fatalf("Invalid value for --read-write: %s", readWriteProvider)
		}

		runStorageSvc(port, readWriteConfig, readOnlyConfigs)
	}

	select {}
}
