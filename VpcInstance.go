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
	"strings"

	"github.com/IBM/vpc-go-sdk/vpcv1"
)

type VpcInstance struct {
	name             string

	groupID          string

	services         *Services

	innerVpcInstance *vpcv1.Instance
}

const (
	vpciObjectName = "Cloud VM"
)

func NewVpcInstance(services *Services) ([]RunnableObject, []error) {
	var (
		vpcInstanceName string
		resourceGroupID string
		vpcSvc          *vpcv1.VpcV1
		ctx             context.Context
		cancel          context.CancelFunc
		foundInstances  []string
		vpcis           []RunnableObject
		errs            []error
		err             error
	)

	vpcInstanceName, err = services.GetMetadata().GetObjectName(RunnableObject(&VpcInstance{}))
	if err != nil {
		return []RunnableObject{&VpcInstance{
			name:             vpcInstanceName,
			services:         services,
			innerVpcInstance: nil,
		}}, []error{ err }
	}

        resourceGroupID = services.GetMetadata().GetResourceGroup()
        log.Debugf("NewVpcInstance: resourceGroupID = %s", resourceGroupID)

	resourceGroupID, err = services.ResourceGroupNameToID(resourceGroupID)
	if err != nil {
		return []RunnableObject{&VpcInstance{
			name:             vpcInstanceName,
			services:         services,
			innerVpcInstance: nil,
		}}, []error{ err }
	}

	vpcSvc = services.GetVpcSvc()

	ctx, cancel = services.GetContextWithTimeout()
	defer cancel()

	if fUseTagSearch {
		foundInstances, err = listByTag(TagTypeCloudInstance, services)
	} else {
		foundInstances, err = findVPCInstancesByName(vpcSvc, ctx, vpcInstanceName, resourceGroupID)
	}
	if err != nil {
		return []RunnableObject{&VpcInstance{
			name:             vpcInstanceName,
			services:         services,
			innerVpcInstance: nil,
		}}, []error{ err }
	}
	log.Debugf("NewVpcInstance: foundInstances = %+v, err = %v", foundInstances, err)

	vpcis = make([]RunnableObject, 0)
	errs = make([]error, 0)

	if len(foundInstances) == 0 {
		// Return an empty array of objects
		return vpcis, errs
	}

	for _, instanceID := range foundInstances {
		var (
			getInstanceOptions *vpcv1.GetInstanceOptions
			instance           *vpcv1.Instance
			instanceName       string
		)

		getInstanceOptions = vpcSvc.NewGetInstanceOptions(instanceID)

		instance, _, err = vpcSvc.GetInstanceWithContext(ctx, getInstanceOptions)
		if err != nil {
			// Just in case.
			instance = nil
		}

		if instance != nil {
			instanceName = *instance.Name
		} else {
			instanceName = vpcInstanceName
		}
		vpcis = append(vpcis, &VpcInstance{
			name:             instanceName,
			services:         services,
			innerVpcInstance: instance,
		})
		errs = append(errs, err)
	}

	return vpcis, errs
}

func findVPCInstancesByName(vpcSvc *vpcv1.VpcV1, ctx context.Context, name string, groupID string) ([]string, error) {
	var (
		listOptions    *vpcv1.ListInstancesOptions
		pager          *vpcv1.InstancesPager
		allInstances   []vpcv1.Instance
		instance       vpcv1.Instance
		foundInstances []string
		err            error
	)

	log.Debugf("findVPCInstancesByName: name = %s", name)
	log.Debugf("findVPCInstancesByName: groupID = %s", groupID)

	listOptions = vpcSvc.NewListInstancesOptions()
	listOptions.SetResourceGroupID(groupID)

	pager, err = vpcSvc.NewInstancesPager(listOptions)
	if err != nil {
		log.Fatalf("Error: findVPCInstancesByName: NewInstancesPager returns %v", err)
		return nil, err
	}

	allInstances, err = pager.GetAllWithContext(ctx)
	if err != nil {
		return nil, err
	}

	foundInstances = make([]string, 0)

	for _, instance = range allInstances {
		if !strings.Contains(*instance.Name, name) {
			log.Debugf("findVPCInstancesByName: SKIP %s %s", *instance.Name, *instance.HealthState)
			continue
		}

		log.Debugf("findVPCInstancesByName: FOUND %s %s", *instance.Name, *instance.HealthState)
		foundInstances = append(foundInstances, *instance.ID)
	}

	return foundInstances, nil
}

func (vpci *VpcInstance) CRN() (string, error) {
	if vpci.innerVpcInstance == nil || vpci.innerVpcInstance.CRN == nil {
		return "(error)", nil
	}

	return *vpci.innerVpcInstance.CRN, nil
}

func (vpci *VpcInstance) Name() (string, error) {
	if vpci.innerVpcInstance == nil || vpci.innerVpcInstance.CRN == nil {
		return "(error)", nil
	}

	return *vpci.innerVpcInstance.Name, nil
}

func (vpci *VpcInstance) ObjectName() (string, error) {
	return vpciObjectName, nil
}

func (vpci *VpcInstance) Run() error {
	return nil
}

func (vpci *VpcInstance) CiStatus(shouldClean bool) {
}

func (vpci *VpcInstance) ClusterStatus() {
	if vpci.innerVpcInstance == nil {
		fmt.Printf("%s is NOTOK. Could not find a %s named %s\n", vpciObjectName, vpciObjectName, vpci.name)
		return
	}

	switch *vpci.innerVpcInstance.HealthState {
	case "ok":
//	case "degraded":
//	case "faulted":
//	case "inapplicable":
//	case "failed":
//	case "deleting":
	default:
		fmt.Printf("%s %s is NOTOK.  The health state is not ok but %s\n", vpciObjectName, vpci.name, *vpci.innerVpcInstance.HealthState)
		return
	}

	fmt.Printf("%s %s is OK.\n", vpciObjectName, vpci.name)
}

func (vpci *VpcInstance) Priority() (int, error) {
	return 85, nil
}
