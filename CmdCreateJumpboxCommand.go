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
	"reflect"
	"strings"

	"github.com/IBM/vpc-go-sdk/vpcv1"

	"github.com/sirupsen/logrus"
)

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
