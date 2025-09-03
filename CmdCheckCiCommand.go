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
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
)

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
