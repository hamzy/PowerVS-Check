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

package main

import (
	"context"
	"fmt"
	gohttp "net/http"
	"regexp"
	"strings"

	"github.com/IBM/go-sdk-core/v5/core"

	"github.com/IBM/platform-services-go-sdk/resourcecontrollerv2"

	"github.com/IBM-Cloud/power-go-client/clients/instance"
	// https://github.com/IBM-Cloud/power-go-client/tree/master/clients/instance
	// https://raw.githubusercontent.com/IBM-Cloud/power-go-client/refs/heads/master/clients/instance/ibm-pi-instance.go
	"github.com/IBM-Cloud/power-go-client/ibmpisession"
	"github.com/IBM-Cloud/power-go-client/power/models"
	// https://github.com/IBM-Cloud/power-go-client/tree/master/power/models
	// https://raw.githubusercontent.com/IBM-Cloud/power-go-client/refs/heads/master/power/models/p_vm_instance.go
)

const (
	serviceInstanceTypeName = "service instance"

	// resource ID for Power Systems Virtual Server in the Global catalog.
	virtualServerResourceID = "abd259f0-9990-11e8-acc8-b9f54a8f1661"

	siObjectName = "Power Service Instance"
)

type ServiceInstance struct {
	name           string
	dhcpName       string
	rhcosName      string
	sshKeyName     string
	networkName    string
	services       *Services
	innerSi        *resourcecontrollerv2.ResourceInstance
	piSession      *ibmpisession.IBMPISession
	networkClient  *instance.IBMPINetworkClient
	innerNetwork   *models.Network
	keyClient      *instance.IBMPIKeyClient
	innerSshKey    *models.SSHKey
	imageClient    *instance.IBMPIImageClient
	stockImageId   string
	rhcosImageId   string
	dhcpClient     *instance.IBMPIDhcpClient
	dhcpServer     *models.DHCPServerDetail
	instanceClient *instance.IBMPIInstanceClient
	jobClient      *instance.IBMPIJobClient
}

func NewServiceInstance(services *Services) ([]RunnableObject, []error) {
	var (
		siName          string
		infraID         string
		dhcpName        string
		rhcosName       string
		sshKeyName      string
		networkName     string
		resourceGroupID string
		guid            string
		controllerSvc   *resourcecontrollerv2.ResourceControllerV2
		ctx             context.Context
		cancel          context.CancelFunc
		foundInstances  []string
		si              *ServiceInstance
		sis             []RunnableObject
		errs            []error
		idxSi           int
		err             error
	)

	infraID = services.GetMetadata().GetInfraID()
	dhcpName = fmt.Sprintf("DHCPSERVER%s", infraID)
	rhcosName = fmt.Sprintf("rhcos-%s", infraID)
	sshKeyName = fmt.Sprintf("%s-sshkey", infraID)
	networkName = fmt.Sprintf("%s-network", infraID)

	si = &ServiceInstance{
		name:        "",
		dhcpName:    dhcpName,
		rhcosName:   rhcosName,
		sshKeyName:  sshKeyName,
		networkName: networkName,
		services:    services,
		innerSi:     nil,
	}

	siName, err = services.GetMetadata().GetObjectName(RunnableObject(&ServiceInstance{}))
	if err != nil {
		return []RunnableObject{si}, []error{err}
	}

	resourceGroupID = services.GetResourceGroupID()
	log.Debugf("NewServiceInstance: resourceGroupID = %s", resourceGroupID)

	guid = services.GetMetadata().GetServiceInstanceGUID()
	log.Debugf("NewServiceInstance: guid = %s", guid)

	controllerSvc = services.GetControllerSvc()

	ctx, cancel = services.GetContextWithTimeout()
	defer cancel()

	if fUseTagSearch {
		foundInstances, err = listByTag(TagTypeServiceInstance, services)
	} else {
		if guid != "" {
			foundInstances, err = findServiceInstance(controllerSvc, ctx, guid, resourceGroupID)
		} else {
			foundInstances, err = findServiceInstance(controllerSvc, ctx, siName, resourceGroupID)
		}
	}
	log.Debugf("NewServiceInstance: foundInstances = %+v, err = %v", foundInstances, err)

	sis = make([]RunnableObject, 1)
	errs = make([]error, 1)

	idxSi = 0
	sis[idxSi] = si

	if len(foundInstances) == 0 {
		if guid != "" {
			errs[idxSi] = fmt.Errorf("Unable to find %s with a guid of %s", siObjectName, guid)
		} else {
			errs[idxSi] = fmt.Errorf("Unable to find %s named %s", siObjectName, siName)
		}
	}

	log.Debugf("NewServiceInstance: len(foundInstances) = %d", len(foundInstances))
	for _, instanceID := range foundInstances {
		var (
			getResourceOptions *resourcecontrollerv2.GetResourceInstanceOptions
			innerResource      *resourcecontrollerv2.ResourceInstance
			response           *core.DetailedResponse
			innerResourceName  string
		)

		log.Debugf("NewServiceInstance: instanceID = %s", instanceID)

		getResourceOptions = controllerSvc.NewGetResourceInstanceOptions(instanceID)

		innerResource, response, err = controllerSvc.GetResourceInstanceWithContext(ctx, getResourceOptions)
		if err != nil {
			err = fmt.Errorf("%s failed to get instance %s: %s: %v", siObjectName, instanceID, response, err)
		}

		if response != nil && response.StatusCode == gohttp.StatusNotFound {
			err = fmt.Errorf("%s gohttp.StatusNotFound for %s", siObjectName, instanceID)
			innerResource = nil
		} else if response != nil && response.StatusCode == gohttp.StatusInternalServerError {
			err = fmt.Errorf("%s gohttp.StatusInternalServerError for %s", siObjectName, instanceID)
			innerResource = nil
		}

		if innerResource.Type == nil {
			err = fmt.Errorf("%s has type nil for %s", siObjectName, instanceID)
			innerResource = nil
		} else {
			log.Debugf("NewServiceInstance: type: %v", *innerResource.Type)
			if *innerResource.Type != "service_instance" && *innerResource.Type != "composite_instance" {
				err = fmt.Errorf("%s has unknown type %s for %s", siObjectName, *innerResource.Type, instanceID)
				innerResource = nil
			}
		}

		if innerResource.GUID == nil {
			err = fmt.Errorf("%s has guid nil for %s", siObjectName, instanceID)
			innerResource = nil
		}

		if err != nil {
			// Just in case.
			innerResource = nil
		}

		if innerResource != nil {
			innerResourceName = *innerResource.Name
		} else {
			innerResourceName = siName
		}
		si.name = innerResourceName
		si.innerSi = innerResource

		err = createClients(si)
		if err != nil {
			si.innerSi = nil
		}

		if idxSi > 0 {
			log.Debugf("NewServiceInstance: appending to sis")

			sis = append(sis, si)
			errs = append(errs, err)
		} else {
			log.Debugf("NewServiceInstance: altering first sis")

			sis[idxSi] = si
			errs[idxSi] = err
		}

		idxSi++
	}

	return sis, errs
}

// findServiceInstance returns the service instance matching by name in the IBM Cloud.
func findServiceInstance(controllerSvc *resourcecontrollerv2.ResourceControllerV2, ctx context.Context, name string, resourceGroupID string) ([]string, error) {
	var (
		options   *resourcecontrollerv2.ListResourceInstancesOptions
		resources *resourcecontrollerv2.ResourceInstancesList
		perPage   int64 = 10
		moreData        = true
		nextURL   *string
		err       error
	)

	log.Debugf("Listing service instances by NAME %s", name)

	matchFunc := func(ri resourcecontrollerv2.ResourceInstance, match string) bool {
		if match == "" {
			return false
		}
		if strings.Contains(*ri.Name, match) {
			return true
		}
		if *ri.GUID == match {
			return true
		}
		if *ri.CRN == match {
			return true
		}
		return false
	}

	options = controllerSvc.NewListResourceInstancesOptions()
	// options.SetType("resource_instance")
	options.SetResourceGroupID(resourceGroupID)
	options.SetResourceID(virtualServerResourceID)
	options.SetLimit(perPage)

	for moreData {
		select {
		case <-ctx.Done():
			log.Debugf("listServiceInstancesByName: case <-ctx.Done()")
			return nil, ctx.Err() // we're cancelled, abort
		default:
		}

		if options.Start != nil {
			log.Debugf("listServiceInstancesByName: options = %+v, options.Limit = %v, options.Start = %v, options.ResourceGroupID = %v", options, *options.Limit, *options.Start, *options.ResourceGroupID)
		} else {
			log.Debugf("listServiceInstancesByName: options = %+v, options.Limit = %v, options.ResourceGroupID = %v", options, *options.Limit, *options.ResourceGroupID)
		}

		resources, _, err = controllerSvc.ListResourceInstancesWithContext(ctx, options)
		if err != nil {
			return nil, fmt.Errorf("failed to list resource instances: %w", err)
		}

		log.Debugf("listServiceInstancesByName: resources.RowsCount = %v", *resources.RowsCount)

		for _, resource := range resources.Resources {
			select {
			case <-ctx.Done():
				log.Debugf("listServiceInstancesByName: case <-ctx.Done()")
				return nil, ctx.Err() // we're cancelled, abort
			default:
			}

			if !matchFunc(resource, name) {
				log.Debugf("listServiceInstancesByName: SKIP  resource.Name = %s", *resource.Name)
				log.Debugf("listServiceInstancesByName: SKIP  resource.CRN = %s", *resource.CRN)
				continue
			}

			log.Debugf("listServiceInstancesByName: FOUND resource.Name = %s", *resource.Name)

			return []string{*resource.ID}, nil
		}

		// Based on: https://cloud.ibm.com/apidocs/resource-controller/resource-controller?code=go#list-resource-instances
		nextURL, err = core.GetQueryParam(resources.NextURL, "start")
		if err != nil {
			return nil, fmt.Errorf("failed to GetQueryParam on start: %w", err)
		}
		if nextURL == nil {
			log.Debugf("nextURL = nil")
			options.SetStart("")
		} else {
			log.Debugf("nextURL = %v", *nextURL)
			options.SetStart(*nextURL)
		}

		moreData = *resources.RowsCount == perPage
	}

	return []string{}, nil
}

func createClients(si *ServiceInstance) error {
	var (
		piSession *ibmpisession.IBMPISession
		err       error
	)

	if si.piSession == nil {
		piSession, err = createPiSession(si)
		if err != nil {
			log.Fatalf("Error: createPiSession returns %v", err)
			return err
		}
		log.Debugf("createClients: piSession = %+v", piSession)
		si.piSession = piSession
	}
	if si.piSession == nil {
		return fmt.Errorf("Error: createClients has a nil piSession!")
	}

	if si.networkClient == nil {
		si.networkClient = instance.NewIBMPINetworkClient(context.Background(), si.piSession, *si.innerSi.GUID)
		log.Debugf("createClients: networkClient = %v", si.networkClient)
	}
	if si.networkClient == nil {
		return fmt.Errorf("Error: createClients has a nil networkClient!")
	}

	if si.keyClient == nil {
		si.keyClient = instance.NewIBMPIKeyClient(context.Background(), si.piSession, *si.innerSi.GUID)
		log.Debugf("createClients: keyClient = %v", si.keyClient)
	}
	if si.keyClient == nil {
		return fmt.Errorf("Error: createClients has a nil keyClient!")
	}

	if si.imageClient == nil {
		si.imageClient = instance.NewIBMPIImageClient(context.Background(), si.piSession, *si.innerSi.GUID)
		log.Debugf("createClients: imageClient = %v", si.imageClient)
	}
	if si.imageClient == nil {
		return fmt.Errorf("Error: createClients has a nil imageClient!")
	}

	if si.dhcpClient == nil {
		si.dhcpClient = instance.NewIBMPIDhcpClient(context.Background(), si.piSession, *si.innerSi.GUID)
		log.Debugf("createClients: dhcpClient = %v", si.dhcpClient)
	}
	if si.dhcpClient == nil {
		return fmt.Errorf("Error: createClients has a nil dhcpClient!")
	}

	if si.instanceClient == nil {
		si.instanceClient = instance.NewIBMPIInstanceClient(context.Background(), si.piSession, *si.innerSi.GUID)
		log.Debugf("createClients: instanceClient = %v", si.instanceClient)
	}
	if si.instanceClient == nil {
		return fmt.Errorf("Error: createClients has a nil instanceClient!")
	}

	if si.jobClient == nil {
		si.jobClient = instance.NewIBMPIJobClient(context.Background(), si.piSession, *si.innerSi.GUID)
		log.Debugf("createClients: jobClient = %v", si.jobClient)
	}
	if si.jobClient == nil {
		return fmt.Errorf("Error: createClients has a nil jobClient!")
	}

	return nil
}

func createPiSession(si *ServiceInstance) (*ibmpisession.IBMPISession, error) {
	var (
		metadata      *Metadata
		authenticator *core.IamAuthenticator
		piOptions     *ibmpisession.IBMPIOptions
		piSession     *ibmpisession.IBMPISession
		err           error
	)

	metadata = si.services.GetMetadata()

	authenticator = &core.IamAuthenticator{
		ApiKey: si.services.GetApiKey(),
	}
	err = authenticator.Validate()
	if err != nil {
		return nil, err
	}

	log.Debugf("createPiSession: region = %+v", metadata.GetRegion())
	piOptions = &ibmpisession.IBMPIOptions{
		Authenticator: authenticator,
		Debug:         false,
		Region:        metadata.GetRegion(),
		URL:           fmt.Sprintf("https://%s.power-iaas.cloud.ibm.com", metadata.GetRegion()),
		UserAccount:   si.services.GetUser().Account,
		Zone:          metadata.GetZone(),
	}
	log.Debugf("createPiSession: piOptions = %+v", piOptions)

	piSession, err = ibmpisession.NewIBMPISession(piOptions)
	if err != nil {
		return nil, fmt.Errorf("Error ibmpisession.New: %v", err)
	}
	log.Debugf("createPiSession: piSession = %v", piSession)

	return piSession, nil
}

func (si *ServiceInstance) FindDhcpServer() (*models.DHCPServerDetail, error) {
	var (
		dhcpServers      []*models.DHCPServer
		dhcpServer       *models.DHCPServer
		dhcpServerDetail *models.DHCPServerDetail
		err              error
	)

	if si.innerSi == nil {
		return nil, fmt.Errorf("Error: findDhcpServer called on nil ServiceInstance")
	}
	if si.dhcpClient == nil {
		return nil, fmt.Errorf("Error: findDhcpServer has nil dhcpClient")
	}

	dhcpServers, err = si.GetDhcpServers()
	if err != nil {
		return nil, err
	}

	log.Debugf("FindDhcpServer: si.dhcpName = %s", si.dhcpName)

	for _, dhcpServer = range dhcpServers {
		if strings.Contains(*dhcpServer.Network.Name, si.dhcpName) {
			log.Debugf("FindDhcpServer: FOUND %s %s", *dhcpServer.ID, *dhcpServer.Network.Name)

			dhcpServerDetail, err = si.dhcpClient.Get(*dhcpServer.ID)
			if err != nil {
				return nil, fmt.Errorf("Error: si.dhcpClient.Get returns %v", err)
			}

			return dhcpServerDetail, nil
		}
		log.Debugf("FindDhcpServer: SKIP %s %s", *dhcpServer.ID, *dhcpServer.Network.Name)
	}

	return nil, nil
}

func (si *ServiceInstance) GetDhcpServers() ([]*models.DHCPServer, error) {
	var (
		dhcpServers models.DHCPServers
		dhcpServer  *models.DHCPServer
		result      []*models.DHCPServer
		err         error
	)

	if si.innerSi == nil {
		return nil, fmt.Errorf("Error: getDhcpServers called on nil ServiceInstance")
	}
	if si.dhcpClient == nil {
		return nil, fmt.Errorf("Error: getDhcpServers has nil dhcpClient")
	}

	dhcpServers, err = si.dhcpClient.GetAll()
	if err != nil {
		return nil, fmt.Errorf("Error: si.dhcpClient.GetAll returns %v", err)
	}

	result = make([]*models.DHCPServer, 0)

	for _, dhcpServer = range dhcpServers {
		if dhcpServer.ID == nil {
			log.Debugf("GetDhcpServers: SKIP nil(ID)")
			continue
		}
		if dhcpServer.Network == nil {
			log.Debugf("GetDhcpServers: SKIP %s nil(Network)", *dhcpServer.ID)
			continue
		}
		if dhcpServer.Network.Name == nil {
			log.Debugf("GetDhcpServers: SKIP %s nil(Network.Name)", *dhcpServer.ID)
			continue
		}
		result = append(result, dhcpServer)
	}

	return result, nil
}

func (si *ServiceInstance) FindImage(imageName string) (*models.ImageReference, error) {
	var (
		imageRefs []*models.ImageReference
		imageRef  *models.ImageReference
		err       error
	)

	if si.innerSi == nil {
		return nil, fmt.Errorf("Error: findImage called on nil ServiceInstance")
	}
	if si.imageClient == nil {
		return nil, fmt.Errorf("Error: findImage has nil imageClient")
	}

	imageRefs, err = si.GetImages()
	if err != nil {
		return nil, err
	}

	for _, imageRef = range imageRefs {
		if strings.Contains(*imageRef.Name, imageName) {
			log.Debugf("FindImage: FOUND EXISTING %s %s", *imageRef.Name, *imageRef.State)
			return imageRef, nil
		} else {
			log.Debugf("FindImage: SKIP EXISTING %s %s", *imageRef.Name, *imageRef.State)
			continue
		}
	}

	return nil, nil
}

func (si *ServiceInstance) GetImages() ([]*models.ImageReference, error) {
	var (
		images *models.Images
		err    error
	)

	if si.innerSi == nil {
		return nil, fmt.Errorf("Error: getImages called on nil ServiceInstance")
	}
	if si.imageClient == nil {
		return nil, fmt.Errorf("Error: getImages has nil imageClient")
	}

	images, err = si.imageClient.GetAll()
	if err != nil {
		log.Fatalf("Error: GetImages: GetAll returns %v", err)
		return nil, err
	}
	log.Debugf("GetImages: images = %+v", images)

	return images.Images, nil
}

func (si *ServiceInstance) FindStockImage(imageName string) (*models.ImageReference, error) {
	var (
		images   *models.Images
		imageRef *models.ImageReference
		err      error
	)

	images, err = si.imageClient.GetAllStockImages(false, false)
	if err != nil {
		log.Fatalf("Error: FindStockImage: GetAllStockImages returns %v", err)
		return nil, err
	}

	for _, imageRef = range images.Images {
		if *imageRef.Name != imageName || *imageRef.State != "active" {
			log.Debugf("FindStockImage: SKIP STOCK %s %s", *imageRef.Name, *imageRef.State)
			continue
		}

		if *imageRef.Name == imageName && *imageRef.State == "active" {
			log.Debugf("FindStockImage: FOUND STOCK %s %s %s", *imageRef.Name, *imageRef.State, *imageRef.ImageID)
			return imageRef, nil
		}
	}

	return nil, nil
}

func (si *ServiceInstance) FindStockImages() ([]string, error) {
	var (
		images   *models.Images
		imageRef *models.ImageReference
		result   = make([]string, 0)
		err      error
	)

	images, err = si.imageClient.GetAllStockImages(false, false)
	if err != nil {
		log.Fatalf("Error: FindStockImages: GetAllStockImages returns %v", err)
		return nil, err
	}

	for _, imageRef = range images.Images {
		if *imageRef.State != "active" {
			log.Debugf("FindStockImages: SKIP    STOCK %s %s", *imageRef.Name, *imageRef.State)
			continue
		} else if *imageRef.State == "active" {
			log.Debugf("FindStockImages: FOUND   STOCK %s %s %s", *imageRef.Name, *imageRef.State, *imageRef.ImageID)
			result = append(result, *imageRef.Name)
		} else {
			log.Debugf("FindStockImages: UNKNOWN STOCK %s %s", *imageRef.Name, *imageRef.State)
			continue
		}
	}

	return result, nil
}

func (si *ServiceInstance) FindPVMInstance(pvmInstanceName string) ([]*models.PVMInstance, error) {
	var (
		instanceRefs     []*models.PVMInstanceReference
		instanceRef      *models.PVMInstanceReference
		matchExp         *regexp.Regexp
		matchedInstances []*models.PVMInstance
		instance         *models.PVMInstance
		err              error
	)

	if si.innerSi == nil {
		return nil, fmt.Errorf("Error: FindPVMInstance called on nil ServiceInstance")
	}
	if si.instanceClient == nil {
		return nil, fmt.Errorf("Error: FindPVMInstance has nil instanceClient")
	}

	instanceRefs, err = si.GetPVMInstances()
	if err != nil {
		return nil, err
	}

	log.Debugf("FindPVMInstance: pvmInstanceName = %s", pvmInstanceName)
	matchExp = regexp.MustCompile(pvmInstanceName)

	matchFunc := func(instanceRef *models.PVMInstanceReference, match string) bool {
		if match == "" {
			return false
		}
		if matchExp.MatchString(*instanceRef.ServerName) {
			return true
		}
		if *instanceRef.PvmInstanceID == match {
			return true
		}
		return false
	}

	matchedInstances = make([]*models.PVMInstance, 0)

	log.Debugf("FindPVMInstance: len(instanceRefs) = %d", len(instanceRefs))
	for _, instanceRef = range instanceRefs {
		if matchFunc(instanceRef, pvmInstanceName) {
			log.Debugf("FindPVMInstance: FOUND %s %s", *instanceRef.ServerName, *instanceRef.PvmInstanceID)

			instance, err = si.instanceClient.Get(*instanceRef.PvmInstanceID)
			if err != nil {
				log.Fatalf("Error: findPVMInstance: GetAll returns %v", err)
				return nil, err
			}

			matchedInstances = append(matchedInstances, instance)
		}

		log.Debugf("FindPVMInstance: SKIP  %s %s", *instanceRef.ServerName, *instanceRef.PvmInstanceID)
	}

	return matchedInstances, nil
}

func (si *ServiceInstance) GetPVMInstances() ([]*models.PVMInstanceReference, error) {
	var (
		instances *models.PVMInstances
		err       error
	)

	if si.innerSi == nil {
		return nil, fmt.Errorf("Error: GetPVMInstances called on nil ServiceInstance")
	}
	if si.instanceClient == nil {
		return nil, fmt.Errorf("Error: GetPVMInstances has nil instanceClient")
	}

	instances, err = si.instanceClient.GetAll()
	if err != nil {
		log.Fatalf("Error: GetPVMInstances: GetAll returns %v", err)
		return nil, err
	}

	return instances.PvmInstances, nil
}

func (si *ServiceInstance) FindSshKey() (*models.SSHKey, error) {
	var (
		keys *models.SSHKeys
		key  *models.SSHKey
		err  error
	)

	if si.innerSi == nil {
		return nil, fmt.Errorf("Error: FindSshKey called on nil ServiceInstance")
	}
	if si.keyClient == nil {
		return nil, fmt.Errorf("Error: FindSshKey has nil keyClient")
	}

	log.Debugf("FindSshKey: si.sshKeyName = %s", si.sshKeyName)

	keys, err = si.keyClient.GetAll()
	if err != nil {
		return nil, fmt.Errorf("Error: FindSshKey: si.keyClient.GetAll returns %v", err)
	}

	for _, key = range keys.SSHKeys {
		if strings.Contains(*key.Name, si.sshKeyName) {
			log.Debugf("FindSshKey: FOUND: %s", *key.Name)

			key, err = si.keyClient.Get(*key.Name)
			if err != nil {
				return nil, fmt.Errorf("Error: FindSshKey: si.keyClient.Get(%s) returns %v", *key.Name, err)
			}

			return key, nil
		} else {
			log.Debugf("FindSshKey: SKIP: %s", *key.Name)
		}
	}

	return nil, nil
}

func (si *ServiceInstance) FindNetwork() (*models.Network, error) {
	var (
		networkRefs []*models.NetworkReference
		networkRef  *models.NetworkReference
		network     *models.Network
		err         error
	)

	if si.innerSi == nil {
		return nil, fmt.Errorf("Error: FindNetwork called on nil ServiceInstance")
	}
	if si.networkClient == nil {
		return nil, fmt.Errorf("Error: FindNetwork has nil networkClient")
	}

	log.Debugf("FindNetwork: si.networkName = %s", si.networkName)

	networkRefs, err = si.GetNetworks()
	if err != nil {
		return nil, err
	}

	for _, networkRef = range networkRefs {
		if strings.Contains(*networkRef.Name, si.networkName) {
			log.Debugf("FindNetwork: FOUND: %s, %s", *networkRef.NetworkID, *networkRef.Name)

			network, err = si.networkClient.Get(*networkRef.NetworkID)
			if err != nil {
				return nil, fmt.Errorf("Error: FindNetwork: si.networkClient.Get(%s) returns %v", *networkRef.NetworkID, err)
			}

			return network, nil
		} else {
			log.Debugf("FindNetwork: SKIP: %s, %s", *networkRef.NetworkID, *networkRef.Name)
		}
	}

	return nil, nil
}

func (si *ServiceInstance) GetNetworks() ([]*models.NetworkReference, error) {
	var (
		networks *models.Networks
		err      error
	)

	if si.innerSi == nil {
		return nil, fmt.Errorf("Error: GetNetworks called on nil ServiceInstance")
	}
	if si.networkClient == nil {
		return nil, fmt.Errorf("Error: GetNetworks has nil networkClient")
	}

	networks, err = si.networkClient.GetAll()
	if err != nil {
		return nil, fmt.Errorf("Error: GetNetworks: si.networkClient.GetAll returns %v", err)
	}

	return networks.Networks, nil
}

func (si *ServiceInstance) GetNetworkPorts(networkID string) ([]*models.NetworkPort, error) {
	var (
		networkPorts *models.NetworkPorts
		err          error
	)

	if si.innerSi == nil {
		return nil, fmt.Errorf("Error: GetNetworkPorts called on nil ServiceInstance")
	}
	if si.networkClient == nil {
		return nil, fmt.Errorf("Error: GetNetworkPorts has nil networkClient")
	}

	networkPorts, err = si.networkClient.GetAllPorts(networkID)
	if err != nil {
		return nil, fmt.Errorf("Error: GetNetworkPorts: si.networkClient.GetAllPorts returns %v", err)
	}

	return networkPorts.Ports, nil
}

func (si *ServiceInstance) GetNetworkInterfaces(networkID string) ([]*models.NetworkInterface, error) {
	var (
		networkInterfaces *models.NetworkInterfaces
		err               error
	)

	if si.innerSi == nil {
		return nil, fmt.Errorf("Error: GetNetworkInterfaces called on nil ServiceInstance")
	}
	if si.networkClient == nil {
		return nil, fmt.Errorf("Error: GetNetworkInterfaces has nil networkClient")
	}

	networkInterfaces, err = si.networkClient.GetAllNetworkInterfaces(networkID)
	if err != nil {
		return nil, fmt.Errorf("Error: GetNetworkInterfaces: si.networkClient.GetNetworkInterfacesAll returns %v", err)
	}

	return networkInterfaces.Interfaces, nil
}

func (si *ServiceInstance) GetSshKeyname() string {
	if si.innerSi == nil {
		return ""
	}

	return si.sshKeyName
}

func (si *ServiceInstance) CRN() (string, error) {
	if si.innerSi == nil || si.innerSi.CRN == nil {
		return "(error)", nil
	}

	return *si.innerSi.CRN, nil
}

func (si *ServiceInstance) Name() (string, error) {
	if si.innerSi == nil || si.innerSi.Name == nil {
		return "(error)", nil
	}

	return *si.innerSi.Name, nil
}

func (si *ServiceInstance) ObjectName() (string, error) {
	return siObjectName, nil
}

func (si *ServiceInstance) Run() error {
	// Nothing needs to be done!
	return nil
}

func (si *ServiceInstance) CiStatus(shouldClean bool) {
	var (
		isOk         = true
		dhcpServers  []*models.DHCPServer
		imageRefs    []*models.ImageReference
		networkRefs  []*models.NetworkReference
		instanceRefs []*models.PVMInstanceReference
		err          error
	)

	instanceRefs, err = si.GetPVMInstances()
	if err != nil {
		fmt.Printf("%s %s returned this error searching for instances: %v\n", siObjectName, si.name, err)
		isOk = false
	}

	log.Debugf("CiStatus: instanceRefs = %+v", instanceRefs)
	if len(instanceRefs) > 0 {
		fmt.Printf("%s %s is NOTOK. Found %d instances.\n", siObjectName, si.name, len(instanceRefs))
		isOk = false

		if shouldClean {
			for _, instanceRef := range instanceRefs {
				err = si.instanceClient.Delete(*instanceRef.PvmInstanceID)
				if err != nil {
					fmt.Printf("%s %s returned this error deleting instance %s: %v\n", siObjectName, si.name, *instanceRef.PvmInstanceID, err)
				}
			}
		}
	}

	dhcpServers, err = si.GetDhcpServers()
	if err != nil {
		fmt.Printf("%s %s returned this error searching for DHCP servers: %v\n", siObjectName, si.name, err)
		isOk = false
	}

	log.Debugf("CiStatus: dhcpServers = %+v", dhcpServers)
	if len(dhcpServers) > 0 {
		dhcps := make([]string, 0)
		for _, dhcpServer := range dhcpServers {
			dhcps = append(dhcps, *dhcpServer.Network.Name)
		}

		fmt.Printf("%s %s is NOTOK. Found %d DHCP servers (%+v).\n", siObjectName, si.name, len(dhcpServers), dhcps)
		isOk = false

		if shouldClean {
			for _, dhcpServer := range dhcpServers {
				err = si.dhcpClient.Delete(*dhcpServer.ID)
				if err != nil {
					fmt.Printf("%s %s returned this error deleting DHCP server %s: %v\n", siObjectName, si.name, *dhcpServer.ID, err)
				}
			}
		}
	}

	imageRefs, err = si.GetImages()
	if err != nil {
		fmt.Printf("%s %s returned this error searching for images: %v\n", siObjectName, si.name, err)
		isOk = false
	}

	log.Debugf("CiStatus: imageRefs = %+v", imageRefs)
	if len(imageRefs) > 0 {
		images := make([]string, 0)
		for _, imageRef := range imageRefs {
			images = append(images, *imageRef.Name)
		}

		fmt.Printf("%s %s is NOTOK. Found %d images (%+v).\n", siObjectName, si.name, len(imageRefs), images)
		isOk = false

		if shouldClean {
			for _, imageRef := range imageRefs {
				err = si.imageClient.Delete(*imageRef.ImageID)
				if err != nil {
					fmt.Printf("%s %s returned this error deleting image %s: %v\n", siObjectName, si.name, *imageRef.ImageID, err)
				}
			}
		}
	}

	networkRefs, err = si.GetNetworks()
	if err != nil {
		fmt.Printf("%s %s returned this error searching for networks: %v\n", siObjectName, si.name, err)
		isOk = false
	}

	log.Debugf("CiStatus: networkRefs = %+v", networkRefs)
	if len(networkRefs) > 0 {
		networks := make([]string, 0)
		for _, networkRef := range networkRefs {
			networks = append(networks, *networkRef.Name)
		}

		fmt.Printf("%s %s is NOTOK. Found %d networks (%+v).\n", siObjectName, si.name, len(networkRefs), networks)
		isOk = false

		for _, networkRef := range networkRefs {
			networkPorts, err := si.GetNetworkPorts(*networkRef.NetworkID)
			if err != nil {
				fmt.Printf("%s %s returned this error calling GetNetworkPorts(%s) %v\n", siObjectName, si.name, *networkRef.NetworkID, err)
				continue
			}

			fmt.Printf("%s %s Network %s has %d NetworkPorts\n", siObjectName, si.name, *networkRef.Name, len(networkPorts))
			for _, networkPort := range networkPorts {
				if networkPort.PvmInstance != nil {
					var (
						serverName = networkPort.PvmInstance.PvmInstanceID
					)

					instancesFound, _ := si.FindPVMInstance(serverName)
					if len(instancesFound) == 1 {
						serverName = *instancesFound[0].ServerName
					}

					fmt.Printf("%s %s Found a server instance (%s) on the network\n", siObjectName, si.name, serverName)

					if shouldClean {
						err = si.instanceClient.Delete(networkPort.PvmInstance.PvmInstanceID)
						if err != nil {
							fmt.Printf("%s %s returned this error deleting instance %s: %v\n", siObjectName, si.name, networkPort.PvmInstance.PvmInstanceID, err)
						}
					}
				}
			}
		}

		if shouldClean {
			for _, networkRef := range networkRefs {
				err = si.networkClient.Delete(*networkRef.NetworkID)
				if err != nil {
					fmt.Printf("%s %s returned this error deleting network %s: %v\n", siObjectName, si.name, *networkRef.NetworkID, err)
				}
			}
		}
	}

	if !isOk {
		return
	}

	fmt.Printf("%s %s is OK.\n", siObjectName, si.name)
}

func (si *ServiceInstance) ClusterStatus() {
	var (
		isOk = true
	)

	if si.innerSi == nil {
		fmt.Printf("%s is NOTOK. Could not find a %s named %s\n", siObjectName, siObjectName, si.name)
		return
	}

	if *si.innerSi.State != "active" {
		fmt.Printf("%s %s is NOTOK. The status is %s\n", siObjectName, si.name, *si.innerSi.State)
		return
	}

	dhcpServer, err := si.FindDhcpServer()
	log.Debugf("dhcpServer = %+v, err = %v", dhcpServer, err)
	if err == nil && dhcpServer != nil {
		fmt.Printf("%s %s has a DHCP server.\n", siObjectName, si.name)
	} else {
		fmt.Printf("%s %s is NOTOK.  Did not find a DHCP server.\n", siObjectName, si.name)
		isOk = false
	}
	if err != nil {
		fmt.Printf("%s %s returned this error searching for DHCP servers: %v\n", siObjectName, si.name, err)
		isOk = false
	}

	dhcpServers, err := si.GetDhcpServers()
	if err != nil {
	} else if len(dhcpServers) > 1 {
		fmt.Printf("%s %s is NOTOK.  Found more than 1 DHCP server (%d).\n", siObjectName, si.name, len(dhcpServers))
		isOk = false
	}

	imageRHCOS, err := si.FindImage(si.rhcosName)
	log.Debugf("imageRHCOS = %+v, err = %v", imageRHCOS, err)
	if err == nil {
		if imageRHCOS != nil {
			if *imageRHCOS.State == "active" {
				fmt.Printf("%s %s has an active RHCOS image.\n", siObjectName, si.name)
			} else {
				fmt.Printf("%s %s does not have an active RHCOS image. (%s)\n", siObjectName, si.name, *imageRHCOS.State)
				isOk = false
			}
		} else {
			fmt.Printf("%s %s something went wrong looking for the RHCOS image.\n", siObjectName, si.name)
			isOk = false
		}
	}
	if err != nil {
		fmt.Printf("%s %s returned this error searching for images: %v\n", siObjectName, si.name, err)
		isOk = false
	}

	sshKey, err := si.FindSshKey()
	log.Debugf("sshKey = %+v, err = %v", sshKey, err)
	if err == nil {
		fmt.Printf("%s %s has an ssh key.\n", siObjectName, si.name)
	} else {
		fmt.Printf("%s %s returned this error searching for ssh keys: %v\n", siObjectName, si.name, err)
		isOk = false
	}

	for i := 0; i < 3; i++ {
		mastersFound, err := si.FindPVMInstance(fmt.Sprintf("%s-.*-master-%d", si.services.GetMetadata().GetClusterName(), i))
		log.Debugf("mastersFound = %+v, err = %v", mastersFound, err)
		if err != nil {
			fmt.Printf("%s %s did not have a master-%d instance got error: %v\n", siObjectName, si.name, i, err)
			isOk = false
		} else if len(mastersFound) == 1 {
			log.Debugf("findPVMInstance master[%d].Status = %s", i, *mastersFound[0].Status)
			log.Debugf("findPVMInstance master[%d].Health.Status = %s", i, mastersFound[0].Health.Status)

			if *mastersFound[0].Status == "ACTIVE" {
				fmt.Printf("%s %s found a healthy master-%d instance (status: %s, health: %s).\n", siObjectName, si.name, i, *mastersFound[0].Status, mastersFound[0].Health.Status)
			} else {
				fmt.Printf("%s %s found an unhealthy master-%d instance (status: %s, health: %s).\n", siObjectName, si.name, i, *mastersFound[0].Status, mastersFound[0].Health.Status)
				isOk = false
			}
		} else {
			fmt.Printf("%s %s did not have 1 master-%d instance, found %d.\n", siObjectName, si.name, i, len(mastersFound))
			isOk = false
		}
	}

	workersFound, err := si.FindPVMInstance(fmt.Sprintf("%s-.*-worker-", si.services.GetMetadata().GetClusterName()))
	if err != nil {
		fmt.Printf("%s %s did not have a worker instance got error: %v\n", siObjectName, si.name, err)
		isOk = false
	} else if len(workersFound) > 0 {
		fmt.Printf("%s %s found %d worker instances.\n", siObjectName, si.name, len(workersFound))

		for _, worker := range workersFound {
			log.Debugf("findPVMInstance worker.Status = %s", *worker.Status)
			log.Debugf("findPVMInstance worker.Health.Status = %s", worker.Health.Status)

			if *worker.Status == "ACTIVE" {
				fmt.Printf("%s %s found a healthy worker instance %s (status: %s, health: %s).\n", siObjectName, si.name, *worker.ServerName, *worker.Status, worker.Health.Status)
			} else {
				fmt.Printf("%s %s found an unhealthy worker instance %s (status: %s, health: %s).\n", siObjectName, si.name, *worker.ServerName, *worker.Status, worker.Health.Status)
				isOk = false
			}
		}
	} else {
		fmt.Printf("%s %s did not find any worker instances.\n", siObjectName, si.name)
		isOk = false
	}

	if !isOk {
		fmt.Printf("%s %s is NOTOK.\n", siObjectName, si.name)
		return
	}

	fmt.Printf("%s %s is OK.\n", siObjectName, si.name)
}

func (si *ServiceInstance) Priority() (int, error) {
	return 80, nil
}
