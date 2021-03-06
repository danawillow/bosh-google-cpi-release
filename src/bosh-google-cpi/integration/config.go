package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"

	"bosh-google-cpi/action"
	boshapi "bosh-google-cpi/api"
	boshdisp "bosh-google-cpi/api/dispatcher"
	"bosh-google-cpi/api/transport"
	boshcfg "bosh-google-cpi/config"
	"bosh-google-cpi/google/client"

	boshlogger "github.com/cloudfoundry/bosh-utils/logger"
	"github.com/cloudfoundry/bosh-utils/uuid"
)

const (
	reusableVMName = "google-cpi-int-tests"
)

var (
	// A stemcell that will be created in integration_suite_test.go
	existingStemcell string

	// Provided by user
	googleProject    = os.Getenv("GOOGLE_PROJECT")
	externalStaticIP = os.Getenv("EXTERNAL_STATIC_IP")
	keepResuableVM   = os.Getenv("KEEP_REUSABLE_VM")
	stemcellURL      = os.Getenv("STEMCELL_URL")
	stemcellFile     = os.Getenv("STEMCELL_FILE")
	serviceAccount   = os.Getenv("SERVICE_ACCOUNT")

	// Configurable defaults
	networkName          = envOrDefault("NETWORK_NAME", "cfintegration")
	customNetworkName    = envOrDefault("CUSTOM_NETWORK_NAME", "cfintegration-custom")
	customSubnetworkName = envOrDefault("CUSTOM_SUBNETWORK_NAME", "cfintegration-custom-us-central1")
	ipAddrs              = strings.Split(envOrDefault("PRIVATE_IP", "192.168.100.102,192.168.100.103,192.168.100.104"), ",")
	targetPool           = envOrDefault("TARGET_POOL", "cfintegration")
	backendService       = envOrDefault("BACKEND_SERVICE", "cfintegration")
	instanceGroup        = envOrDefault("BACKEND_SERVICE", "cfintegration")
	zone                 = envOrDefault("ZONE", "us-central1-a")
	region               = envOrDefault("REGION", "us-central1")

	// Channel that will be used to retrieve IPs to use
	ips chan string

	// If true, CPI will not wait for delete to complete. Speeds up tests significantly.
	asyncDelete = envOrDefault("CPI_ASYNC_DELETE", "true")

	cfgContent = fmt.Sprintf(`{
	  "cloud": {
		"plugin": "google",
		"properties": {
		  "google": {
			"project": "%v"
		  },
		  "agent": {
			"mbus": "http://127.0.0.1",
			"blobstore": {
			  "provider": "local"
			}
		  },
		  "registry": {
			"use_gce_metadata": true
		  }
		}
	  }
	}`, googleProject)
)

func toggleAsyncDelete() {
	key := "CPI_ASYNC_DELETE"
	current := os.Getenv(key)
	if current == "" {
		os.Setenv(key, "true")
	} else {
		os.Setenv(key, "")
	}
}

func execCPI(request string) (boshdisp.Response, error) {
	var err error
	var cfg boshcfg.Config
	var in, out, errOut, errOutLog bytes.Buffer
	var boshResponse boshdisp.Response
	var googleClient client.GoogleClient

	if cfg, err = boshcfg.NewConfigFromString(cfgContent); err != nil {
		return boshResponse, err
	}

	multiWriter := io.MultiWriter(&errOut, &errOutLog)
	logger := boshlogger.NewWriterLogger(boshlogger.LevelDebug, multiWriter, multiWriter)
	multiLogger := boshapi.MultiLogger{Logger: logger, LogBuff: &errOutLog}
	uuidGen := uuid.NewGenerator()
	if googleClient, err = client.NewGoogleClient(cfg.Cloud.Properties.Google, multiLogger); err != nil {
		return boshResponse, err
	}

	actionFactory := action.NewConcreteFactory(
		googleClient,
		uuidGen,
		cfg,
		multiLogger,
	)

	caller := boshdisp.NewJSONCaller()
	dispatcher := boshdisp.NewJSON(actionFactory, caller, multiLogger)

	in.WriteString(request)
	cli := transport.NewCLI(&in, &out, dispatcher, multiLogger)

	var response []byte

	if err = cli.ServeOnce(); err != nil {
		return boshResponse, err
	}

	if response, err = ioutil.ReadAll(&out); err != nil {
		return boshResponse, err
	}

	if err = json.Unmarshal(response, &boshResponse); err != nil {
		return boshResponse, err
	}
	return boshResponse, nil
}

func envOrDefault(key, defaultVal string) (val string) {
	if val = os.Getenv(key); val == "" {
		val = defaultVal
	}
	return
}
