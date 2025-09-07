// Copyright 2025 IBM Corp
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// (/bin/rm go.*; go mod init example/user/PowerVS-Check; go mod tidy)
// (echo "vet:"; go vet || exit 1; echo "build:"; go build -ldflags="-X main.version=$(git describe --always --long --dirty) -X main.release=$(git describe --tags --abbrev=0)" -o PowerVS-Check-Create *.go || exit 1; echo "run:"; ./PowerVS-Check check-create -apiKey "..." -metadata metadata.json -shouldDebug true)

package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rivo/tview"

	"github.com/sirupsen/logrus"
)

const (
	useTview = false
	useSavedJson = false
)

func watchCreateCommand(watchCreateClusterFlags *flag.FlagSet, args []string) error {
	var (
		out            io.Writer
		ptrApiKey      *string
		ptrShouldDebug *string
		ptrInstallDir  *string
		err            error
	)

	ptrApiKey = watchCreateClusterFlags.String("apiKey", "", "Your IBM Cloud API key")
	ptrShouldDebug = watchCreateClusterFlags.String("shouldDebug", "false", "Should output debug output")
	ptrInstallDir = watchCreateClusterFlags.String("installDir", "", "The KUBECONFIG file")

	watchCreateClusterFlags.Parse(args)

	switch strings.ToLower(*ptrShouldDebug) {
	case "true":
		shouldDebug = true
	case "false":
		shouldDebug = false
	default:
		return fmt.Errorf("Error: shouldDebug is not true/false (%s)\n", *ptrShouldDebug)
	}

	if shouldDebug {
		out = os.Stderr
	} else {
		out = io.Discard
	}
	log = &logrus.Logger{
		Out:       out,
		Formatter: new(logrus.TextFormatter),
		Level:     logrus.DebugLevel,
	}

	if *ptrApiKey == "" {
		return fmt.Errorf("Error: No API key set, use -apiKey")
	}

	// Before we do a lot of work, validate the apikey!
	_, err = InitBXService(*ptrApiKey)
	if err != nil {
		return err
	}

	if *ptrInstallDir == "" {
		return fmt.Errorf("Error: No installation directory set, use -installDir")
	}

	fmt.Fprintf(os.Stderr, "Program version is %v, release = %v\n", version, release)

	kubeconfigCapi := filepath.Join(*ptrInstallDir, ".clusterapi_output/envtest.kubeconfig")

	if _, err = os.Stat(kubeconfigCapi); errors.Is(err, os.ErrNotExist) && !useSavedJson {
		return err
	}

	err = watchCAPIPhases(kubeconfigCapi)
	if err != nil {
		return err
	}

	err = watchOpenshiftPhases(*ptrInstallDir, *ptrApiKey)
	if err != nil {
		return err
	}

	return nil
}

func updateWindow(capiWindows map[string]*tview.TextView, element string, text string) {
	if useTview {
		capiWindows[element].SetText(text)
	} else {
		fmt.Println(text)
	}
}

func updateCAPIPhase1(kubeconfig string, app *tview.Application, capiWindows map[string]*tview.TextView, chanResult chan<- error) {
	var (
		cmdOcGetPVSCluster = []string{
			"oc", "get", "ibmpowervscluster", "-n", "openshift-cluster-api-guests", "-o", "json",
		}
		jsonPVSCluster     map[string]interface{}
		aconditions        []statusCondition
		clusterReady       bool
		ok                 bool
		conditionsReady    bool
		err                error
	)

	for true {
		if useSavedJson {
			jsonPVSCluster, err = parseJsonFile("ibmpowervscluster2.json")
		} else {
			jsonPVSCluster, err = runSplitCommandJson(kubeconfig, cmdOcGetPVSCluster)
		}
		if err != nil {
			err = fmt.Errorf("Error: could not run command: %v", err)
			break
		}
//		log.Debugf("updateCAPIPhase1: jsonPVSCluster = %+v", jsonPVSCluster)

		// https://medium.com/@sumit-s/mastering-go-channels-from-beginner-to-pro-9c1eaba0da9e

		// @TODO is there a way to avoid the large hardcoded value?
		bufferedChannel := make(chan error, 100)

		aconditions, clusterReady = getPVSCluster(jsonPVSCluster, bufferedChannel)

		err = gatherBufferedErrors(bufferedChannel)
		if err != nil {
			continue
		}

		if !useTview {
			fmt.Println("Querying the IBMPowerVSCluster: 8<--------8<--------")
		}

		conditionsReady = true
		for _, condition := range aconditions {
			if useTview {
				_, ok = capiWindows[condition.Type]
				log.Debugf("updateCAPIPhase1: condition = %+v, ok = %v", condition, ok)
				if !ok {
					continue
				}
			}
			if condition.Status {
				updateWindow(capiWindows, condition.Type, fmt.Sprintf("%s is READY", condition.Type))
			} else {
				conditionsReady = false
				updateWindow(capiWindows, condition.Type, fmt.Sprintf("%s is NOT READY", condition.Type))
			}
		}

		if conditionsReady {
			log.Debugf("updateCAPIPhase1: conditionsReady = %v, len(aconditions) = %d", conditionsReady, len(aconditions))
			log.Debugf("updateCAPIPhase1: clusterReady = %v", clusterReady)
			if clusterReady {
				fmt.Println("Cluster is READY")
			} else {
				fmt.Println("Cluster is NOT READY")
			}

			if len(aconditions) == 8 && clusterReady {
				err = nil
				break
			}
		} else {
			log.Debugf("updateCAPIPhase1: conditionsReady = %v", conditionsReady)
		}

		time.Sleep(10 * time.Second)
	}

	if useTview {
		app.Stop()
	}

	chanResult <- err

	log.Debugf("updateCAPIPhase1: DONE!")
}

func updateCAPIPhase2(kubeconfig string, app *tview.Application, capiWindows map[string]*tview.TextView, chanResult chan<- error) {
	var (
		cmdOcGetPVSImage = []string{
			"oc", "get", "ibmpowervsimage", "-n", "openshift-cluster-api-guests", "-o", "json",
		}
		jsonPVSImage       map[string]interface{}
		aconditions        []statusCondition
		conditionsReady    bool
		err                error
	)

	for true {
		if useSavedJson {
			jsonPVSImage, err = parseJsonFile("ibmpowervsimage1.json")
		} else {
			jsonPVSImage, err = runSplitCommandJson(kubeconfig, cmdOcGetPVSImage)
		}
		if err != nil {
			err = fmt.Errorf("Error: could not run command: %v", err)
			break
		}
//		log.Debugf("updateCAPIPhase2: jsonPVSImage = %+v", jsonPVSImage)

		// https://medium.com/@sumit-s/mastering-go-channels-from-beginner-to-pro-9c1eaba0da9e

		// @TODO is there a way to avoid the large hardcoded value?
		bufferedChannel := make(chan error, 100)

		aconditions = getPVSImage(jsonPVSImage, bufferedChannel)

		err = gatherBufferedErrors(bufferedChannel)
		if err != nil {
			continue
		}

		if !useTview {
			fmt.Println("Querying the IBMPowerVSImage: 8<--------8<--------")
		}

		conditionsReady = true
		for _, condition := range aconditions {
			log.Debugf("updateCAPIPhase2: condition = %+v", condition)
			if condition.Status {
				updateWindow(capiWindows, condition.Type, fmt.Sprintf("%s is READY", condition.Type))
			} else {
				conditionsReady = false
				updateWindow(capiWindows, condition.Type, fmt.Sprintf("%s is NOT READY", condition.Type))
			}
		}

		if conditionsReady {
			log.Debugf("updateCAPIPhase2: conditionsReady = %v, len(aconditions) = %d", conditionsReady, len(aconditions))
			if len(aconditions) == 2 {
				err = nil
				break
			}
		} else {
			log.Debugf("updateCAPIPhase2: conditionsReady = %v", conditionsReady)
		}

		time.Sleep(10 * time.Second)
	}

	if useTview {
		app.Stop()
	}

	chanResult <- err

	log.Debugf("updateCAPIPhase2: DONE!")
}

func updateCAPIPhase3(kubeconfig string, app *tview.Application, capiWindows map[string]*tview.TextView, chanResult chan<- error) {
	var (
		cmdOcGetPVSMachines = []string{
			"oc", "get", "ibmpowervsmachines", "-n", "openshift-cluster-api-guests", "-o", "json",
		}
		jsonPVSMachines    map[string]interface{}
		aconditions        []statusCondition
		conditionsReady    bool
		err                error
	)

	for true {
		if useSavedJson {
			jsonPVSMachines, err = parseJsonFile("ibmpowervsmachines2.json")
		} else {
			jsonPVSMachines, err = runSplitCommandJson(kubeconfig, cmdOcGetPVSMachines)
		}
		if err != nil {
			err = fmt.Errorf("Error: could not run command: %v", err)
			break
		}
//		log.Debugf("updateCAPIPhase3: jsonPVSMachines = %+v", jsonPVSMachines)

		// https://medium.com/@sumit-s/mastering-go-channels-from-beginner-to-pro-9c1eaba0da9e

		// @TODO is there a way to avoid the large hardcoded value?
		bufferedChannel := make(chan error, 100)

		aconditions = getPVSMachines(jsonPVSMachines, bufferedChannel)

		err = gatherBufferedErrors(bufferedChannel)
		if err != nil {
			continue
		}

		if !useTview {
			fmt.Println("Querying the IBMPowerVSMachines: 8<--------8<--------")
		}

		conditionsReady = true
		for _, condition := range aconditions {
			var (
				buf bytes.Buffer
			)

			log.Debugf("updateCAPIPhase3: condition = %+v", condition)

			fmt.Fprintf(&buf, "%s is ", condition.Name)
			if condition.Status {
				fmt.Fprintf(&buf, "READY")
			} else {
				conditionsReady = false
				fmt.Fprintf(&buf, "NOT READY")
			}

			if condition.Address == "" {
				conditionsReady = false
				fmt.Fprintf(&buf, ", address is empty")
			} else {
				fmt.Fprintf(&buf, ", address is %s", condition.Address)
			}

			fmt.Println(buf.String())
		}

		if conditionsReady {
			log.Debugf("updateCAPIPhase3: conditionsReady = %v, len(aconditions) = %d", conditionsReady, len(aconditions))
			if len(aconditions) == 4 {
				err = nil
				break
			}
		} else {
			log.Debugf("updateCAPIPhase3: conditionsReady = %v", conditionsReady)
		}

		time.Sleep(10 * time.Second)
	}

	if useTview {
		app.Stop()
	}

	chanResult <- err

	log.Debugf("updateCAPIPhase3: DONE!")
}

func watchCAPIPhases(kubeconfig string) error {
	var (
		app         *tview.Application
		grid        *tview.Grid
		windowList   = []string {
			"COSInstanceCreated", "LoadBalancerReady", "NetworkReady", "ServiceInstanceReady", "TransitGatewayReady", "VPCReady", "VPCSecurityGroupReady", "VPCSubnetReady",
	}
		capiWindows map[string]*tview.TextView
		chanResult  chan error
		err         error
	)

	newTextView := func(name string) *tview.TextView {
		tv := tview.NewTextView()
		tv.SetText(fmt.Sprintf("%s is unknown", name))
		return tv
	}

	if useTview {
		app = tview.NewApplication()
		grid = tview.NewGrid().SetBorders(true)
	}

	capiWindows = make(map[string]*tview.TextView)

	if useTview {
		for _, name := range windowList {
			capiWindows[name] = newTextView(name)
		}

		position := 1
		for _, name := range windowList {
			grid.AddItem(capiWindows[name], position, 0, 1, 1, 0, 0, false)
			position += 1
		}
	}

	chanResult = make(chan error)

	if useTview {
		go updateCAPIPhase1(kubeconfig, app, capiWindows, chanResult)

//		time.Sleep(15*time.Second)

		if err = app.SetRoot(grid, true).SetFocus(grid).Run(); err != nil {
			return err
		}
	} else {
		go updateCAPIPhase1(kubeconfig, app, capiWindows, chanResult)
		log.Debugf("watchCAPIPhase: updateCAPIPhase1: chan = %+v", <-chanResult)

		go updateCAPIPhase2(kubeconfig, app, capiWindows, chanResult)
		log.Debugf("watchCAPIPhase: updateCAPIPhase2: chan = %+v", <-chanResult)

		go updateCAPIPhase3(kubeconfig, app, capiWindows, chanResult)
		log.Debugf("watchCAPIPhase: updateCAPIPhase3: chan = %+v", <-chanResult)
	}

	return nil
}

func printStatus(status string, label string, printSpace bool) {
	switch status {
	case "True":
		fmt.Printf("%s", label)
	case "False":
		fmt.Printf("NOT %s", label)
	case "":
		fmt.Printf("(EMPTY) %s", label)
	default:
		fmt.Printf("(ERROR %s) %s", status, label)
	}
	if printSpace {
		fmt.Printf(", ")
	}
}

func watchOpenshiftPhases(installDir string, apiKey string) error {
	var (
		metadataLocation    string
		kubeconfigOpenshift string
		err                 error
	)

	metadataLocation = filepath.Join(installDir, "metadata.json")
	kubeconfigOpenshift = filepath.Join(installDir, "auth/kubeconfig")

	if _, err = os.Stat(metadataLocation); errors.Is(err, os.ErrNotExist) {
		return err
	}
	if _, err = os.Stat(kubeconfigOpenshift); errors.Is(err, os.ErrNotExist) && !useSavedJson {
		return err
	}

	for _, phase := range []func(installDir string, apiKey string) error {
		updateOpenshiftPhase1,
		updateOpenshiftPhase2,
		updateOpenshiftPhase3,
		updateOpenshiftPhase99,
	} {
		err = phase(installDir, apiKey)
		if err != nil {
			return err
		}
	}

	return nil
}

func updateOpenshiftPhase1(installDir string, apiKey string) error {
	var (
		metadata       *Metadata
		services       *Services
		errs           []error
		aloadBalancers []*LoadBalancer
		intLb          *LoadBalancer
		err            error
	)

	metadataLocation := filepath.Join(installDir, "metadata.json")

	metadata, err = NewMetadataFromCCMetadata(metadataLocation)
	if err != nil {
		return err
	}

	services, err = NewServices(metadata, apiKey)
	if err != nil {
		return fmt.Errorf("Error: Could not create a Services object (%s)!\n", err)
	}

	fmt.Fprintf(os.Stderr, "Querying the Load Balancer...\n")

	aloadBalancers, errs = NewLoadBalancerAlt(services)
	log.Debugf("aloadBalancers = %+v", aloadBalancers)
	log.Debugf("errs = %+v", errs)

	// Loop through the returned errors.
	for _, err = range errs {
		if err != nil {
			return err
		}
	}

	for _, lb := range aloadBalancers {
		name, err := lb.Name()
		if err == nil {
			switch GetLoadBalancerType(name) {
			case LoadBalancerTypeInternal:
				intLb = lb
			case LoadBalancerTypeExternal:
			}
		}
	}

	for true {
		if intLb != nil {
			if intLb.CheckLoadBalancerPool([]string{"machine-config-server", "additional-pool-22623"}, "machine config server") {
				break
			}
		}
		time.Sleep(10 * time.Second)
	}

	return nil
}

func updateOpenshiftPhase2(installDir string, apiKey string) error {
	return updateOpenshiftPhaseClusterOperator(installDir, apiKey, "network")
}

func updateOpenshiftPhase3(installDir string, apiKey string) error {
	var (
		cmdOcGetDeployment = []string{
			"oc", "get", "deployment/powervs-cloud-controller-manager", "-n", "openshift-cloud-controller-manager", "-o", "json",
		}
		jsonOGD            map[string]interface{}
		cc                 clusterConditions
		err                error
	)

	kubeconfigOpenshift := filepath.Join(installDir, "auth/kubeconfig")

	for true {
		if useSavedJson {
			jsonOGD, err = parseJsonFile("ocgetdeploymentpccm1.json")
		} else {
			jsonOGD, err = runSplitCommandJson(kubeconfigOpenshift, cmdOcGetDeployment)
		}
		if err != nil {
			return fmt.Errorf("Error: could not run command: %v", err)
		}
//		log.Debugf("updateOpenshiftPhase2: jsonOGD = %+v", jsonOGD)

		// @TODO is there a way to avoid the large hardcoded value?
		bufferedChannel := make(chan error, 100)

		cc = getDeployment(jsonOGD, bufferedChannel)

		err = gatherBufferedErrors(bufferedChannel)
		if err != nil {
			log.Debugf("updateOpenshiftPhase2: getClusterOperator returns %v", err)
		} else {
			log.Debugf("updateOpenshiftPhase2: cc = %+v", cc)
		}

		fmt.Printf("The deployment of powervs-cloud-controller-manager is: ")
		printStatus(cc.Available, "AVAILABLE", false)
		fmt.Printf("\n")

		if cc.Available == "True" {
			break
		}

		time.Sleep(10 * time.Second)
	}

	return err
}

func updateOpenshiftPhase99(installDir string, apiKey string) error {
	return updateOpenshiftPhaseClusterOperator(installDir, apiKey, "authentication")
}

func updateOpenshiftPhaseClusterOperator(installDir string, apiKey string, operator string) error {
	var (
		cmdOcGetCo = []string{
			"oc", "--request-timeout=5s", "get", "co", "-o", "json",
		}
		jsonCo     map[string]interface{}
		cc         clusterConditions
		err        error
	)

	kubeconfigOpenshift := filepath.Join(installDir, "auth/kubeconfig")

	for true {
		if useSavedJson {
			jsonCo, err = parseJsonFile("ocgetco1.json")
		} else {
			jsonCo, err = runSplitCommandJson(kubeconfigOpenshift, cmdOcGetCo)
		}
		if err != nil {
			return fmt.Errorf("Error: could not run command: %v", err)
		}

		// @TODO is there a way to avoid the large hardcoded value?
		bufferedChannel := make(chan error, 100)

		cc = getClusterOperator(jsonCo, operator, bufferedChannel)

		err = gatherBufferedErrors(bufferedChannel)
		if err != nil {
			log.Debugf("updateOpenshiftPhaseClusterOperator: getClusterOperator returns %v", err)
		} else {
			log.Debugf("updateOpenshiftPhaseClusterOperator: cc = %+v", cc)
		}

		fmt.Printf("The %s cluster operator is: ", operator)
		printStatus(cc.Available, "AVAILABLE", true)
		printStatus(cc.Degraded, "DEGRADED", true)
		printStatus(cc.Progressing, "PROGRESSING", true)
		printStatus(cc.Upgradeable, "UPGRADEABLE", false)
		fmt.Printf("\n")

		if cc.Available == "True" {
			break
		}

		time.Sleep(10 * time.Second)
	}

	return err
}
