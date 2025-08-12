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
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/IBM/vpc-go-sdk/vpcv1"

	"github.com/sirupsen/logrus"
)

var (
	// Replaced with:
	//   -ldflags="-X main.version=$(git describe --always --long --dirty)"
	version = "undefined"
	release = "undefined"

	shouldDebug  = false
	shouldDelete = false

	log *logrus.Logger
)

func printUsage(executableName string) {
	fmt.Fprintf(os.Stderr, "Usage: %s [ "+
		"check-ci | "+
		"check-create | "+
		"check-kubeconfig | "+
		"check-capi-kubeconfig | "+
		"create-jumpbox "+
		"]\n", executableName)
}

func main() {
	var (
		executableName           string
		checkCiFlags             *flag.FlagSet
		checkCreateFlags         *flag.FlagSet
		checkKubeconfigFlags     *flag.FlagSet
		checkCapiKubeconfigFlags *flag.FlagSet
		createJumpboxFlags       *flag.FlagSet
		err                      error
	)

	executablePath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting executable path: %v\n", err)
		os.Exit(1)
	}

	executableName = filepath.Base(executablePath)

	if len(os.Args) == 1 {
		printUsage(executableName)
		os.Exit(1)
	} else if len(os.Args) == 2 && os.Args[1] == "-version" {
		fmt.Fprintf(os.Stderr, "version = %v\nrelease = %v\n", version, release)
		os.Exit(1)
	}

	checkCiFlags = flag.NewFlagSet("check-ci", flag.ExitOnError)
	checkCreateFlags = flag.NewFlagSet("check-create", flag.ExitOnError)
	checkKubeconfigFlags = flag.NewFlagSet("check-kubeconfig", flag.ExitOnError)
	checkCapiKubeconfigFlags = flag.NewFlagSet("check-capi-kubeconfig", flag.ExitOnError)
	createJumpboxFlags = flag.NewFlagSet("create-jumpbox", flag.ExitOnError)

	switch strings.ToLower(os.Args[1]) {
	case "check-ci":
		err = checkCiCommand(checkCiFlags, os.Args[2:])

	case "check-create":
		err = checkCreateCommand(checkCreateFlags, os.Args[2:])

	case "check-kubeconfig":
		err = checkKubeconfigCommand(checkKubeconfigFlags, os.Args[2:])

	case "check-capi-kubeconfig":
		err = checkCapiKubeconfigCommand(checkCapiKubeconfigFlags, os.Args[2:])

	case "create-jumpbox":
		err = createJumpboxCommand(createJumpboxFlags, os.Args[2:])

	default:
		fmt.Fprintf(os.Stderr, "Error: Unknown command %s\n", os.Args[1])
		printUsage(executableName)
		os.Exit(1)
	}

	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func initializeRunnableObjects(services *Services, robjsFuncs []NewRunnableObjectsEntry) ([]RunnableObject, error) {
	var (
		robjsResult    []RunnableObject
		errs           []error
		robjObjectName string
		crnName        string
		robjsCluster   = make([]RunnableObject, 0, 5)
		err            error
	)

	// Loop through New functions which return an array of runnable objects.
	for _, nroe := range robjsFuncs {
		fmt.Fprintf(os.Stderr, "Querying the %s...\n", nroe.Name)

		// Call the New function.
		robjsResult, errs = nroe.NRO(services)

		// Loop through the returned errors.
		for _, err = range errs {
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: Could not create a %s object (%v)!\n", nroe.Name, err)
			}
		}

		// Loop through the array of returned results.
		for _, robj := range robjsResult {
			// What is the runnable object's name?
			robjObjectName, err = robj.ObjectName()
			if err != nil {
				return nil, fmt.Errorf("Error: Could not figure out the objects' name! (%s)\n", err)
			}

			// Also make sure the priority is valid.
			_, err = robj.Priority()
			if err != nil {
				return nil, fmt.Errorf("Error: Could not get the priority for %s: %s\n", robjObjectName, err)
			}

			// Append the runnable object.
			log.Debugf("Appending %s %+v", robjObjectName, robj)
			robjsCluster = append(robjsCluster, robj)

			// What is the runnable object's CRN?
			crnName, err = robj.CRN()
			if err == nil {
				log.Debugf("%s.CRN = %s", robjObjectName, crnName)
			} else {
				log.Debugf("ERROR: %s.CRN: %v", robjObjectName, err)
			}
		}
	}

	// Run each object.
	for _, robj := range robjsCluster {
		robjObjectName, _ = robj.ObjectName()
		fmt.Fprintf(os.Stderr, "Running the %s...\n", robjObjectName)

		err = robj.Run()
		if err != nil {
			return nil, err
		}
	}

	return robjsCluster, nil
}

func checkCiCommand(checkCiFlags *flag.FlagSet, args []string) error {
	var (
		out            io.Writer
		ptrApiKey      *string
		ptrShouldDebug *string
		ptrMetadata    *string
		ptrShouldClean *string
		shouldClean    = false
		metadata       *Metadata
		services       *Services
		robjsFuncs     = []NewRunnableObjectsEntry{
			{NewVpc, "Virtual Private Cloud"},
			{NewTransitGateway, "Transit Gateway"},
			{NewServiceInstance, "Power Service Instance"},
		}
		robjsCluster   []RunnableObject
		robjObjectName string
		err            error
	)

	ptrApiKey = checkCiFlags.String("apiKey", "", "Your IBM Cloud API key")
	ptrShouldDebug = checkCiFlags.String("shouldDebug", "false", "Should output debug output")
	ptrMetadata = checkCiFlags.String("metadata", "", "The location of the metadata.json file")
	ptrShouldClean = checkCiFlags.String("shouldClean", "false", "Should we attempt to clean up?")

	checkCiFlags.Parse(args)

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

	if *ptrMetadata == "" {
		return fmt.Errorf("Error: No metadata file location iset, use -metadata")
	}

	switch strings.ToLower(*ptrShouldClean) {
	case "true":
		shouldClean = true
	case "false":
		shouldClean = false
	default:
		return fmt.Errorf("Error: shouldClean is not true/false (%s)\n", *ptrShouldClean)
	}

	fmt.Fprintf(os.Stderr, "Program version is %v, release = %v\n", version, release)

	// Before we do a lot of work, validate the apikey!
	_, err = InitBXService(*ptrApiKey)
	if err != nil {
		return err
	}

	metadata, err = NewMetadataFromCIMetadata(*ptrMetadata)
	if err != nil {
		return fmt.Errorf("Error: Could not read metadata from %s\n", *ptrMetadata)
	}
	log.Debugf("metadata = %+v", metadata)

	services, err = NewServices(metadata, *ptrApiKey)
	if err != nil {
		return fmt.Errorf("Error: Could not create a Services object (%s)!\n", err)
	}

	robjsCluster, err = initializeRunnableObjects(services, robjsFuncs)
	if err != nil {
		return err
	}

	// Sort the objects by their priority.
	robjsCluster = BubbleSort(robjsCluster)
	for _, robj := range robjsCluster {
		robjObjectName, _ = robj.ObjectName()
		log.Debugf("Sorted %s %+v", robjObjectName, robj)
	}
	fmt.Fprintf(os.Stderr, "Sorted the objects.\n")

	// Query the status of the objects.
	for _, robj := range robjsCluster {
		robj.CiStatus(shouldClean)
	}

	return nil
}

func checkCreateCommand(checkCreateFlags *flag.FlagSet, args []string) error {
	var (
		out            io.Writer
		ptrApiKey      *string
		ptrShouldDebug *string
		ptrMetadata    *string
		metadata       *Metadata
		services       *Services
		robjsFuncs     = []NewRunnableObjectsEntry{
			{NewVpc, "Virtual Private Cloud"},
			{NewCloudObjectStorage, "Cloud Object Storage"},
			{NewTransitGateway, "Transit Gateway"},
			{NewLoadBalancer, "Load Balancer"},
			{NewServiceInstance, "Power Service Instance"},
			{NewVpcInstance, "Cloud VM"},
			{NewDNS, "Domain Name Service"},
		}
		robjsCluster   []RunnableObject
		robjObjectName string
		err            error
	)

	ptrApiKey = checkCreateFlags.String("apiKey", "", "Your IBM Cloud API key")
	ptrShouldDebug = checkCreateFlags.String("shouldDebug", "false", "Should output debug output")
	ptrMetadata = checkCreateFlags.String("metadata", "", "The location of the metadata.json file")

	checkCreateFlags.Parse(args)

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

	if *ptrMetadata == "" {
		return fmt.Errorf("Error: No metadata file location iset, use -metadata")
	}

	fmt.Fprintf(os.Stderr, "Program version is %v, release = %v\n", version, release)

	// Before we do a lot of work, validate the apikey!
	_, err = InitBXService(*ptrApiKey)
	if err != nil {
		return err
	}

	metadata, err = NewMetadataFromCCMetadata(*ptrMetadata)
	if err != nil {
		return fmt.Errorf("Error: Could not read metadata from %s\n", *ptrMetadata)
	}
	log.Debugf("metadata = %+v", metadata)
	log.Debugf("metadata.Region = %s", metadata.GetRegion())

	services, err = NewServices(metadata, *ptrApiKey)
	if err != nil {
		return fmt.Errorf("Error: Could not create a Services object (%s)!\n", err)
	}

	robjsCluster, err = initializeRunnableObjects(services, robjsFuncs)
	if err != nil {
		return err
	}

	// Sort the objects by their priority.
	robjsCluster = BubbleSort(robjsCluster)
	for _, robj := range robjsCluster {
		robjObjectName, _ = robj.ObjectName()
		log.Debugf("Sorted %s %+v", robjObjectName, robj)
	}
	fmt.Fprintf(os.Stderr, "Sorted the objects.\n")

	// Query the status of the objects.
	for _, robj := range robjsCluster {
		robj.ClusterStatus()
	}

	return nil
}

func runCommand(ptrKubeconfig *string, cmdline string) error {
	var (
		acmdline []string
		ctx      context.Context
		cancel   context.CancelFunc
		cmd      *exec.Cmd
		out      []byte
		err      error
	)

	ctx, cancel = context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	// Split the space separated line into an array of strings
	acmdline = strings.Fields(cmdline)

	if len(acmdline) == 0 {
		return fmt.Errorf("runCommand has empty command")
	} else if len(acmdline) == 1 {
		cmd = exec.CommandContext(ctx, acmdline[0])
	} else {
		cmd = exec.CommandContext(ctx, acmdline[0], acmdline[1:]...)
	}

	cmd.Env = append(
		os.Environ(),
		fmt.Sprintf("KUBECONFIG=%s", *ptrKubeconfig),
	)

	fmt.Println("8<--------8<--------8<--------8<--------8<--------8<--------8<--------8<--------")
	fmt.Println(cmdline)
	out, err = cmd.CombinedOutput()
	fmt.Println(string(out))

	return err
}

func runTwoCommands(ptrKubeconfig *string, cmdline1 string, cmdline2 string) error {
	var (
		acmdline1 []string
		acmdline2 []string
		ctx       context.Context
		cancel    context.CancelFunc
		cmd1      *exec.Cmd
		cmd2      *exec.Cmd
		readPipe  *os.File
		writePipe *os.File
		buffer    bytes.Buffer
		out       []byte
		err       error
	)

	ctx, cancel = context.WithTimeout(context.Background(), defaultTimeout)
	defer cancel()

	log.Debugf("cmdline1 = %s", cmdline1)
	log.Debugf("cmdline2 = %s", cmdline2)

	// Split the space separated line into an array of strings
	acmdline1 = strings.Fields(cmdline1)
	acmdline2 = strings.Fields(cmdline2)

	if len(acmdline1) == 0 {
		return fmt.Errorf("runTwoCommands has empty command")
	} else if len(acmdline1) == 1 {
		cmd1 = exec.CommandContext(ctx, acmdline1[0])
	} else {
		cmd1 = exec.CommandContext(ctx, acmdline1[0], acmdline1[1:]...)
	}

	cmd1.Env = append(
		os.Environ(),
		fmt.Sprintf("KUBECONFIG=%s", *ptrKubeconfig),
	)

	if len(acmdline2) == 0 {
		return fmt.Errorf("runTwoCommands has empty command")
	} else if len(acmdline2) == 1 {
		cmd2 = exec.CommandContext(ctx, acmdline2[0])
	} else {
		cmd2 = exec.CommandContext(ctx, acmdline2[0], acmdline2[1:]...)
	}

	readPipe, writePipe, err = os.Pipe()
	if err != nil {
		return fmt.Errorf("Error returned from os.Pipe: %v", err)
	}

	defer readPipe.Close()

	cmd1.Stdout = writePipe

	err = cmd1.Start()
	if err != nil {
		return fmt.Errorf("Error returned from cmd1.Start: %v", err)
	}

	defer cmd1.Wait()

	writePipe.Close()

	cmd2.Stdin = readPipe
	cmd2.Stdout = &buffer
	cmd2.Stderr = &buffer

	cmd2.Run()

	out = buffer.Bytes()

	fmt.Println("8<--------8<--------8<--------8<--------8<--------8<--------8<--------8<--------")
	fmt.Printf("%s | %s\n", cmdline1, cmdline2)
	fmt.Println(string(out))

	return nil
}

func checkKubeconfigCommand(checkKubeconfigFlags *flag.FlagSet, args []string) error {
	var (
		out            io.Writer
		ptrShouldDebug *string
		ptrKubeconfig  *string
		cmds           = []string{
			"oc --request-timeout=5s get clusterversion",
			"oc --request-timeout=5s get co",
			"oc --request-timeout=5s get nodes -o=wide",
			"oc --request-timeout=5s get pods -n openshift-machine-api",
			"oc --request-timeout=5s get machines.machine.openshift.io -n openshift-machine-api",
			"oc --request-timeout=5s get machineset.machine.openshift.io -n openshift-machine-api",
			"oc --request-timeout=5s logs -l k8s-app=controller -c machine-controller -n openshift-machine-api",
			"oc --request-timeout=5s get co/cloud-controller-manager",
			"oc --request-timeout=5s describe cm/cloud-provider-config -n openshift-config",
			"oc --request-timeout=5s get pods -n openshift-cloud-controller-manager-operator",
			"oc --request-timeout=5s get events -n openshift-cloud-controller-manager",
			"oc --request-timeout=5s logs -l k8s-app=powervs-cloud-controller-manager -n openshift-cloud-controller-manager",
			"oc --request-timeout=5s -n openshift-cloud-controller-manager-operator logs deployment/cluster-cloud-controller-manager-operator -c cluster-cloud-controller-manager",
			"oc --request-timeout=5s get co/network",
			"oc --request-timeout=5s get machines.machine.openshift.io -n openshift-machine-api",
			"oc --request-timeout=5s get machineset.m -n openshift-machine-api",
			"oc --request-timeout=5s get pods -n openshift-machine-api",
			"oc --request-timeout=5s describe co/machine-config",
		}
		pipeCmds = [][]string{
			{
				"oc --request-timeout=5s get pods -A -o=wide",
				"sed -e /\\(Running\\|Completed\\)/d",
			},
		}
		err error
	)

	ptrShouldDebug = checkKubeconfigFlags.String("shouldDebug", "false", "Should output debug output")
	ptrKubeconfig = checkKubeconfigFlags.String("kubeconfig", "", "The KUBECONFIG file")

	checkKubeconfigFlags.Parse(args)

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

	if *ptrKubeconfig == "" {
		return fmt.Errorf("Error: No KUBECONFIG key set, use -kubeconfig")
	}

	fmt.Fprintf(os.Stderr, "Program version is %v, release = %v\n", version, release)

	for _, cmd := range cmds {
		err = runCommand(ptrKubeconfig, cmd)
		if err != nil {
			fmt.Printf("Error: could not run command: %v\n", err)
		}
	}

	for _, twoCmds := range pipeCmds {
		err = runTwoCommands(ptrKubeconfig, twoCmds[0], twoCmds[1])
		if err != nil {
			fmt.Printf("Error: could not run command: %v\n", err)
		}
	}

	return nil
}

func checkCapiKubeconfigCommand(checkCapiKubeconfigFlags *flag.FlagSet, args []string) error {
	var (
		out            io.Writer
		ptrShouldDebug *string
		ptrKubeconfig  *string
		cmds           = [][]string{
			{
				"oc get ibmpowervscluster -n openshift-cluster-api-guests -o json",
				"jq -r .items[].status.conditions[]",
			},
			{
				"oc get ibmpowervsimage -n openshift-cluster-api-guests -o json",
				"jq -r .items[].status.conditions[]",
			},
			{
				"oc get ibmpowervsmachines -n openshift-cluster-api-guests -o json",
				"jq -r .items[].status.conditions[]",
			},
		}
		err error
	)

	ptrShouldDebug = checkCapiKubeconfigFlags.String("shouldDebug", "false", "Should output debug output")
	ptrKubeconfig = checkCapiKubeconfigFlags.String("kubeconfig", "", "The KUBECONFIG file")

	checkCapiKubeconfigFlags.Parse(args)

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

	if *ptrKubeconfig == "" {
		return fmt.Errorf("Error: No KUBECONFIG key set, use -kubeconfig")
	}

	fmt.Fprintf(os.Stderr, "Program version is %v, release = %v\n", version, release)

	for _, twoCmds := range cmds {
		err = runTwoCommands(ptrKubeconfig, twoCmds[0], twoCmds[1])
		if err != nil {
			fmt.Printf("Error: could not run command: %v\n", err)
		}
	}

	return nil
}

func createJumpboxCommand(createJumpboxFlags *flag.FlagSet, args []string) error {
	var (
		out            io.Writer
		ptrApiKey      *string
		ptrShouldDebug *string
		ptrMetadata    *string
		ptrImageName   *string
		ptrKeyName     *string
		metadata       *Metadata
		services       *Services
		robjsFuncs     = []NewRunnableObjectsEntry{
			{NewVpc, "Virtual Private Cloud"},
			{NewServiceInstance, "Power Service Instance"},
		}
		robjsCluster    []RunnableObject
		vpc             *Vpc
		si              *ServiceInstance
		name            string
		resourceGroupID string
		imageID         string
		instanceProfile = "bx2d-2x8"
		zone            string
		subnetID        string
		keyID           string
		vpcID           string
		err             error
	)

	ptrApiKey = createJumpboxFlags.String("apiKey", "", "Your IBM Cloud API key")
	ptrShouldDebug = createJumpboxFlags.String("shouldDebug", "false", "Should output debug output")
	ptrMetadata = createJumpboxFlags.String("metadata", "", "The location of the metadata.json file")
	ptrImageName = createJumpboxFlags.String("imageName", "", "The name of the image to use")
	ptrKeyName = createJumpboxFlags.String("keyName", "", "The name of the ssh key to use")

	createJumpboxFlags.Parse(args)

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

	if *ptrMetadata == "" {
		return fmt.Errorf("Error: No metadata file location iset, use -metadata")
	}

	fmt.Fprintf(os.Stderr, "Program version is %v, release = %v\n", version, release)

	// Before we do a lot of work, validate the apikey!
	_, err = InitBXService(*ptrApiKey)
	if err != nil {
		return err
	}

	metadata, err = NewMetadataFromCCMetadata(*ptrMetadata)
	if err != nil {
		return fmt.Errorf("Error: Could not read metadata from %s\n", *ptrMetadata)
	}
	log.Debugf("metadata = %+v", metadata)

	services, err = NewServices(metadata, *ptrApiKey)
	if err != nil {
		return fmt.Errorf("Error: Could not create a Services object (%s)!\n", err)
	}

	robjsCluster, err = initializeRunnableObjects(services, robjsFuncs)
	if err != nil {
		return err
	}
	log.Debugf("robjsCluster = %+v", robjsCluster)

	si = nil
	vpc = nil

	for _, robj := range robjsCluster {
		log.Debugf("reflect.TypeOf(robj).String() = %+v", reflect.TypeOf(robj).String())
		switch reflect.TypeOf(robj).String() {
		case "*main.Vpc":
			vpc = robj.(*Vpc)
		case "*main.ServiceInstance":
			si = robj.(*ServiceInstance)
		}
	}

	if vpc == nil {
		return fmt.Errorf("Error: Could not find VPC!")
	}

	if si == nil {
		return fmt.Errorf("Error: Could not find Service Instance!")
	}

	if *ptrImageName == "" {
		images, err := vpc.ListImages()
		if err != nil {
			return err
		}

		fmt.Println("Please assign -imageName to one of the following:")
		for _, image := range images {
			fmt.Printf("\t%s\n", *image.Name)
		}
		os.Exit(0)
	} else {
		images, err := vpc.ListImages()
		if err != nil {
			return err
		}

		imageID = ""
		for _, image := range images {
			if *ptrImageName == *image.Name {
				imageID = *image.ID
			}
		}
		if imageID == "" {
			return fmt.Errorf("Image (%s) not found! Don't specify -imageName to get a list", *ptrImageName)
		}
	}

	name = fmt.Sprintf("%s-vsi", metadata.GetInfraID())

	resourceGroupID = metadata.GetResourceGroup()
	resourceGroupID, err = services.ResourceGroupNameToID(resourceGroupID)
	if err != nil {
		return err
	}

	zones, err := vpc.GetRegionZones()
	if err != nil {
		return err
	}
	log.Debugf("zones = %+v", zones)
	zone = zones[0]

	subnets, err := vpc.ListSubnets()
	if err != nil {
		return err
	}
	for _, subnet := range subnets {
		if *subnet.Zone.Name == zone {
			subnetID = *subnet.ID
		}
	}

	if *ptrKeyName == "" {
		keys, err := vpc.ListSshKeys()
		if err != nil {
			return err
		}

		fmt.Println("Please assign -keyName to one of the following:")
		for _, key := range keys {
			fmt.Printf("\t%s\n", *key.Name)
		}
		os.Exit(0)
	} else {
		keys, err := vpc.ListSshKeys()
		if err != nil {
			return err
		}

		keyID = ""
		for _, key := range keys {
			if *ptrKeyName == *key.Name {
				keyID = *key.ID
			}
		}
		if keyID == "" {
			return fmt.Errorf("Ssh key (%s) not found! Don't specify -keyName to get a list", *ptrKeyName)
		}
	}

	vpcID, err = vpc.ID()
	if err != nil {
		return err
	}

	log.Debugf("name            = %s", name)
	log.Debugf("resourceGroupID = %s", resourceGroupID)
	log.Debugf("imageID         = %s", imageID)
	log.Debugf("instanceProfile = %s", instanceProfile)
	log.Debugf("zone            = %s", zone)
	log.Debugf("subnetID        = %s", subnetID)
	log.Debugf("keyID           = %s", keyID)
	log.Debugf("vpcID           = %s", vpcID)

	instance, err := vpc.FindInstance(name)
	if err != nil {
		return err
	}
	log.Debugf("instance = %+v", instance)

	if instance == nil {
		fmt.Printf("Creating instance %s\n", name)

		instancePrototype := &vpcv1.InstancePrototypeInstanceByImage{
			Keys: []vpcv1.KeyIdentityIntf{
				&vpcv1.KeyIdentityByID{
					ID: &keyID,
				},
			},
			Name: &name,
			ResourceGroup: &vpcv1.ResourceGroupIdentity{
				ID: &resourceGroupID,
			},
			Profile: &vpcv1.InstanceProfileIdentityByName{
				Name: &instanceProfile,
			},
			VPC: &vpcv1.VPCIdentityByID{
				ID: &vpcID,
			},
			Image: &vpcv1.ImageIdentityByID{
				ID: &imageID,
			},
			PrimaryNetworkInterface: &vpcv1.NetworkInterfacePrototype{
				Name: &name,
				Subnet: &vpcv1.SubnetIdentityByID{
					ID: &subnetID,
				},
			},
			Zone: &vpcv1.ZoneIdentityByName{
				Name: &zone,
			},
		}

		instance, err = vpc.CreateInstance(instancePrototype)
		log.Debugf("instance = %+v", instance)
		log.Debugf("err = %+v", err)
		if err != nil {
			return err
		}
	} else {
		fmt.Printf("Found instance %s\n", name)
	}

	err = vpc.CreateFIP(instance)
	if err != nil {
		return err
	}

	return nil
}
