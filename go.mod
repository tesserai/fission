module github.com/fission/fission

require (
	//github.com/Azure/azure-sdk-for-go v12.4.0-beta+incompatible
	// github.com/Azure/azure-sdk-for-go v25.0.0+incompatible
	github.com/Azure/azure-sdk-for-go v10.2.1-beta+incompatible

	github.com/DataDog/zstd v1.3.5 // indirect
	github.com/Shopify/sarama v1.20.1
	github.com/Shopify/toxiproxy v2.1.4+incompatible // indirect
	github.com/armon/go-metrics v0.0.0-20180917152333-f0300d1749da // indirect
	github.com/blend/go-sdk v1.1.1 // indirect
	github.com/boltdb/bolt v1.3.1 // indirect
	github.com/bsm/sarama-cluster v2.1.15+incompatible
	github.com/dchest/uniuri v0.0.0-20160212164326-8902c56451e9
	github.com/dnaeon/go-vcr v1.0.1 // indirect
	github.com/docker/distribution v2.7.1+incompatible
	github.com/docker/spdystream v0.0.0-20160310174837-449fdfce4d96 // indirect
	github.com/docopt/docopt-go v0.0.0-20160216232012-784ddc588536
	github.com/dsnet/compress v0.0.0-20171208185109-cc9eb1d7ad76 // indirect
	github.com/dustin/go-humanize v1.0.0
	github.com/eapache/go-resiliency v1.1.0 // indirect
	github.com/eapache/go-xerial-snappy v0.0.0-20180814174437-776d5712da21 // indirect
	github.com/eapache/queue v1.1.0 // indirect
	github.com/elazarl/goproxy v0.0.0-20181111060418-2ce16c963a8a // indirect

	github.com/fission/fission/pkg/apis/fission.io v0.0.0
	github.com/fsnotify/fsnotify v1.4.7
	github.com/ghodss/yaml v1.0.0
	github.com/go-sql-driver/mysql v1.4.1 // indirect
	github.com/golang/example v0.0.0-20170904185048-46695d81d1fa
	github.com/golang/freetype v0.0.0-20170609003504-e2365dfdc4a0 // indirect
	github.com/golang/groupcache v0.0.0-20190129154638-5b532d6fd5ef // indirect
	github.com/golang/protobuf v1.2.0
	github.com/golang/snappy v0.0.0-20180518054509-2e65f85255db // indirect
	github.com/gomodule/redigo v0.0.0-20180627144507-2cd21d9966bf
	github.com/google/btree v0.0.0-20180813153112-4030bb1f1f0c // indirect
	github.com/googleapis/gnostic v0.0.0-20170729233727-0c5108395e2d // indirect
	github.com/gophercloud/gophercloud v0.0.0-20180210024343-6da026c32e2d // indirect
	github.com/gorilla/handlers v1.4.0
	github.com/gorilla/mux v1.6.2
	github.com/graymeta/stow v0.0.0
	github.com/gregjones/httpcache v0.0.0-20181110185634-c63ab54fda8f // indirect
	github.com/hashicorp/go-immutable-radix v1.0.0 // indirect
	github.com/hashicorp/go-msgpack v0.5.3 // indirect
	github.com/hashicorp/go-multierror v0.0.0-20180717150148-3d5d8f294aa0
	github.com/hashicorp/raft v1.0.0 // indirect
	github.com/imdario/mergo v0.3.3
	github.com/influxdata/influxdb v1.2.0
	github.com/marstr/guid v0.0.0-20170427235115-8bdf7d1a087c // indirect
	github.com/mholt/archiver v0.0.0-20180417220235-e4ef56d48eb0
	github.com/nats-io/go-nats-streaming v0.4.0
	github.com/nats-io/nats-streaming-server v0.10.2
	github.com/nokia/docker-registry-client v0.0.0-20181128224058-bf401ccb7530 // indirect
	github.com/nwaples/rardecode v0.0.0-20171029023500-e06696f847ae // indirect
	github.com/onsi/ginkgo v1.7.0 // indirect
	github.com/onsi/gomega v1.4.3 // indirect
	github.com/opencontainers/image-spec v1.0.1 // indirect
	github.com/pascaldekloe/goe v0.0.0-20180627143212-57f6aae5913c // indirect
	github.com/peterbourgon/diskv v2.0.1+incompatible // indirect
	github.com/pierrec/lz4 v2.0.2+incompatible // indirect
	github.com/pierrec/xxHash v0.1.1 // indirect
	github.com/pkg/errors v0.8.0
	github.com/prometheus/client_golang v0.9.2
	github.com/prometheus/common v0.0.0-20181126121408-4724e9255275
	github.com/rcrowley/go-metrics v0.0.0-20181016184325-3113b8401b8a // indirect
	github.com/robfig/cron v0.0.0-20180505203441-b41be1df6967
	github.com/satori/go.uuid v1.2.0
	github.com/sirupsen/logrus v0.0.0-20170606205945-68cec9f21fbf
	github.com/stretchr/objx v0.1.1 // indirect
	github.com/stretchr/testify v1.3.0
	github.com/tesserai/docker-registry-client v0.0.0
	github.com/ulikunitz/xz v0.0.0-20180703112113-636d36a76670 // indirect
	github.com/urfave/cli v1.20.0
	github.com/wcharczuk/go-chart v2.0.1+incompatible
	go.opencensus.io v0.18.1-0.20181204023538-aab39bd6a98b
	golang.org/x/image v0.0.0-20181116024801-cd38e8056d9b // indirect
	golang.org/x/net v0.0.0-20190110200230-915654e7eabc
	golang.org/x/time v0.0.0-20161028155119-f51c12702a4d // indirect
	google.golang.org/appengine v1.1.0 // indirect
	gopkg.in/yaml.v2 v2.2.1

	k8s.io/api v0.0.0-20181126151915-b503174bad59
	k8s.io/apiextensions-apiserver v0.0.0-20181126155829-0cd23ebeb688
	k8s.io/apimachinery v0.0.0-20181126123746-eddba98df674
	k8s.io/client-go v0.0.0-20181126152608-d082d5923d3c
)

replace github.com/graymeta/stow => ../stow

replace github.com/fission/fission/pkg/apis/fission.io => ./pkg/apis/fission.io

replace github.com/tesserai/docker-registry-client => ../docker-registry-client
