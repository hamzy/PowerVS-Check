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
	"reflect"
	"strings"

	"github.com/IBM/go-sdk-core/v5/core"
	"github.com/IBM/vpc-go-sdk/vpcv1"

	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/ptr"
)

type Vpc struct {
	name string

	services *Services

	// type VPC struct
	innerVpc *vpcv1.VPC
}

const (
	vpcObjectName = "Virtual Private Cloud"
)

func NewVpc(services *Services) ([]RunnableObject, []error) {
	var (
		vpcs     []*Vpc
		errs     []error
		ros      []RunnableObject
	)

	vpcs, errs = innerNewVpc(services)

	ros = make([]RunnableObject, len(vpcs))
	// Go does not support type converting the entire array.
	// So we do it manually.
	for i, v := range vpcs {
		ros[i] = RunnableObject(v)
	}

	return ros, errs
}

func NewVpcAlt(services *Services) ([]*Vpc, []error) {
	return innerNewVpc(services)
}

func innerNewVpc(services *Services) ([]*Vpc, []error) {
	var (
		vpcName        string
		region         string
		vpcRegion      string
		vpcSvc         *vpcv1.VpcV1
		ctx            context.Context
		cancel         context.CancelFunc
		foundInstances []string
		vpc            *Vpc
		vpcs           []*Vpc
		errs           []error
		idxVpc         int
		err            error
	)

	vpc = &Vpc{
		name:     "",
		services: services,
		innerVpc: nil,
	}

	vpcName, err = services.GetMetadata().GetObjectName(RunnableObject(&Vpc{}))
	if err != nil {
		return []*Vpc{vpc}, []error{err}
	}
	log.Debugf("NewVpc: vpcName = %s", vpcName)
	if vpcName == "" {
		return nil, nil
	}
	vpc.name = vpcName

	region = services.GetMetadata().GetRegion()
	log.Debugf("NewVpc: region = %s", region)

	vpcRegion, err = services.GetMetadata().GetVPCRegion()
	if err != nil {
		return []*Vpc{vpc}, []error{err}
	}
	log.Debugf("NewVpc: vpcRegion = %s", vpcRegion)

	vpcSvc = services.GetVpcSvc()

	ctx, cancel = services.GetContextWithTimeout()
	defer cancel()

	if fUseTagSearch {
		foundInstances, err = listByTag(TagTypeVPC, services)
	} else {
		foundInstances, err = findVpcs(vpcName, vpcSvc, ctx)
	}
	if err != nil {
		return []*Vpc{vpc}, []error{err}
	}
	log.Debugf("NewVpc: foundInstances = %+v, err = %v", foundInstances, err)

	vpcs = make([]*Vpc, 1)
	errs = make([]error, 1)

	idxVpc = 0
	vpcs[idxVpc] = vpc

	if len(foundInstances) == 0 {
		errs[idxVpc] = fmt.Errorf("Unable to find %s named %s", vpcObjectName, vpcName)
	}

	log.Debugf("NewVpc: len(foundInstances) = %d", len(foundInstances))
	for _, instanceID := range foundInstances {
		var (
			getOptions   *vpcv1.GetVPCOptions
			innerVpc     *vpcv1.VPC
			innerVpcName string
		)

		getOptions = vpcSvc.NewGetVPCOptions(instanceID)

		innerVpc, _, err = vpcSvc.GetVPCWithContext(ctx, getOptions)
		if err != nil {
			// Just in case.
			innerVpc = nil
		}

		if innerVpc != nil {
			innerVpcName = *innerVpc.Name
		} else {
			innerVpcName = vpcName
		}
		vpc.name = innerVpcName
		vpc.innerVpc = innerVpc

		if idxVpc > 0 {
			log.Debugf("NewVpc: appending to vpcs")

			vpcs = append(vpcs, vpc)
			errs = append(errs, err)
		} else {
			log.Debugf("NewVpc: altering first vpcs")

			vpcs[idxVpc] = vpc
			errs[idxVpc] = err
		}

		idxVpc++
	}

	return vpcs, errs
}

func findVpcs(name string, vpcSvc *vpcv1.VpcV1, ctx context.Context) ([]string, error) {
	var (
		// type ListVpcsOptions
		options  *vpcv1.ListVpcsOptions
		perPage  int64 = 64
		moreData       = true
		// type VPCCollection
		vpcs           *vpcv1.VPCCollection
		response       *core.DetailedResponse
		foundInstances []string
		err            error
	)
	log.Debugf("findVpcs: name = %s", name)

	matchFunc := func(vpc vpcv1.VPC, match string) bool {
		if match == "" {
			return false
		}
		if strings.Contains(*vpc.Name, match) {
			return true
		}
		if *vpc.CRN == match {
			return true
		}
		return false
	}

	options = vpcSvc.NewListVpcsOptions()
	options.SetLimit(perPage)

	foundInstances = make([]string, 0)

	for moreData {
		vpcs, response, err = vpcSvc.ListVpcsWithContext(ctx, options)
		if err != nil {
			log.Fatalf("Error: findVpcs: ListVpcs: response = %v, err = %v", response, err)
			return nil, err
		}

		for _, currentVpc := range vpcs.Vpcs {
			if !matchFunc(currentVpc, name) {
				log.Debugf("findVpcs: SKIP ID = %s, Name = %s", *currentVpc.ID, *currentVpc.Name)
				continue
			}

			log.Debugf("findVpcs: FOUND ID = %s, Name = %s", *currentVpc.ID, *currentVpc.Name)

			foundInstances = append(foundInstances, *currentVpc.ID)
		}

		if vpcs.Next != nil {
			log.Debugf("findVpcs: Next = %+v", *vpcs.Next)
			start, err := vpcs.GetNextStart()
			if err != nil {
				log.Fatalf("Error: findVpcs: GetNextStart returns %v", err)
				return nil, err
			}
			log.Debugf("findVpcs: start = %+v", *start)
			options.SetStart(*start)
		} else {
			log.Debugf("findVpcs: Next = nil")
			moreData = false
		}
	}

	return foundInstances, nil
}

func (vpc Vpc) ListSubnets() ([]*vpcv1.Subnet, error) {
	var (
		vpcSvc      *vpcv1.VpcV1
		ctx         context.Context
		cancel      context.CancelFunc
		groupID     string
		listOptions *vpcv1.ListSubnetsOptions
		perPage     int64 = 64
		moreData          = true
		subnets     *vpcv1.SubnetCollection
		response    *core.DetailedResponse
		result      []*vpcv1.Subnet
		err         error
	)

	if vpc.innerVpc == nil {
		return nil, fmt.Errorf("ListSubnets: VPC not found!")
	}

	vpcSvc = vpc.services.GetVpcSvc()

	ctx, cancel = vpc.services.GetContextWithTimeout()
	defer cancel()

	groupID = vpc.services.GetMetadata().GetResourceGroup()
	groupID, err = vpc.services.ResourceGroupNameToID(groupID)
	if err != nil {
		return nil, err
	}
	log.Debugf("groupID = %s", groupID)

	listOptions = vpcSvc.NewListSubnetsOptions()
	listOptions.SetLimit(perPage)
	listOptions.SetResourceGroupID(groupID)
	log.Debugf("listOptions = %+v", listOptions)

	result = make([]*vpcv1.Subnet, 0)

	for moreData {
		subnets, response, err = vpcSvc.ListSubnetsWithContext(ctx, listOptions)
		if err != nil {
			log.Fatalf("Error: ListSubnets: ListSubnets: response = %v, err = %v", response, err)
			return nil, err
		}

		for _, subnet := range subnets.Subnets {
			if *subnet.VPC.ID == *vpc.innerVpc.ID {
				log.Debugf("ListSubnets FOUND Name = %s, Ipv4CIDRBlock = %s", *subnet.Name, *subnet.Ipv4CIDRBlock)
				result = append(result, &subnet)
			} else {
				log.Debugf("ListSubnets: SKIP Name = %s, Ipv4CIDRBlock = %s", *subnet.Name, *subnet.Ipv4CIDRBlock)
			}
		}

		if subnets.Next != nil {
			log.Debugf("ListSubnets: Next = %+v", *subnets.Next)
			start, err := subnets.GetNextStart()
			if err != nil {
				log.Fatalf("Error: ListSubnets: GetNextStart returns %v", err)
				return nil, err
			}
			log.Debugf("ListSubnets: start = %+v", *start)
			listOptions.SetStart(*start)
		} else {
			log.Debugf("ListSubnets: Next = nil")
			moreData = false
		}
	}

	return result, nil
}

func (vpc Vpc) FindSecurityGroupsByVPC() ([]string, error) {
	var (
		vpcSvc      *vpcv1.VpcV1
		ctx         context.Context
		cancel      context.CancelFunc
		groupID     string
		getOptions  *vpcv1.GetVPCDefaultSecurityGroupOptions
		defaultSG   *vpcv1.DefaultSecurityGroup
		wantedPorts = sets.New[int64](22, 443, 5000, 6443, 10258, 22623)
		result      []string
		err         error
	)

	if vpc.innerVpc == nil {
		return nil, fmt.Errorf("findSecurityGroupsByVPC: VPC not found!")
	}

	vpcSvc = vpc.services.GetVpcSvc()

	ctx, cancel = vpc.services.GetContextWithTimeout()
	defer cancel()

	groupID = vpc.services.GetMetadata().GetResourceGroup()
	groupID, err = vpc.services.ResourceGroupNameToID(groupID)
	if err != nil {
		return nil, err
	}
	log.Debugf("findSecurityGroupsByVPC: groupID = %s", groupID)

	getOptions = vpcSvc.NewGetVPCDefaultSecurityGroupOptions(*vpc.innerVpc.ID)

	defaultSG, _, err = vpcSvc.GetVPCDefaultSecurityGroupWithContext(ctx, getOptions)
	if err != nil {
		return result, err
	}

	result = make([]string, 0)

	for _, sgRule := range defaultSG.Rules {
		//		log.Debugf("wantedPorts = %+v", wantedPorts)
		//		log.Debugf("findSecurityGroupsByVPC: type = %s", reflect.TypeOf(sgRule).String())
		switch reflect.TypeOf(sgRule).String() {
		case "*vpcv1.SecurityGroupRuleSecurityGroupRuleProtocolAll":
			securityGroupRule, ok := sgRule.(*vpcv1.SecurityGroupRuleSecurityGroupRuleProtocolAll)
			if ok {
				result = append(result, *securityGroupRule.ID)
			}
			//			for i := *securityGroupRule.PortMin; i <= *securityGroupRule.PortMax; i++ {
			//				log.Debugf("SG All deleting %d", i)
			//				wantedPorts.Delete(i)
			//			}
		case "*vpcv1.SecurityGroupRuleSecurityGroupRuleProtocolTcpudp":
			securityGroupRule, ok := sgRule.(*vpcv1.SecurityGroupRuleSecurityGroupRuleProtocolTcpudp)
			if ok {
				result = append(result, *securityGroupRule.ID)
			}
			for i := *securityGroupRule.PortMin; i <= *securityGroupRule.PortMax; i++ {
				log.Debugf("SG Tcpudp deleting %d", i)
				wantedPorts.Delete(i)
			}
		case "*vpcv1.SecurityGroupRuleSecurityGroupRuleProtocolIcmp":
			securityGroupRule, ok := sgRule.(*vpcv1.SecurityGroupRuleSecurityGroupRuleProtocolIcmp)
			if ok {
				result = append(result, *securityGroupRule.ID)
			}
		}
	}
	log.Debugf("wantedPorts = %+v", wantedPorts)

	return result, nil
}

func (vpc *Vpc) FindInstance(name string) (*vpcv1.Instance, error) {
	var (
		vpcSvc      *vpcv1.VpcV1
		ctx         context.Context
		cancel      context.CancelFunc
		groupID     string
		listOptions *vpcv1.ListInstancesOptions
		pager       *vpcv1.InstancesPager
		instances   []vpcv1.Instance
		instance    vpcv1.Instance
		err         error
	)

	log.Debugf("FindInstance: name = %s", name)

	if vpc.innerVpc == nil {
		return nil, fmt.Errorf("FindInstance: VPC not found!")
	}

	vpcSvc = vpc.services.GetVpcSvc()

	ctx, cancel = vpc.services.GetContextWithTimeout()
	defer cancel()

	groupID = vpc.services.GetMetadata().GetResourceGroup()
	groupID, err = vpc.services.ResourceGroupNameToID(groupID)
	if err != nil {
		return nil, err
	}
	log.Debugf("FindInstance: groupID = %s", groupID)

	listOptions = vpcSvc.NewListInstancesOptions()
	listOptions.SetResourceGroupID(groupID)
	//listOptions.SetVPCID(*vpc.innerVpc.ID)

	pager, err = vpcSvc.NewInstancesPager(listOptions)
	if err != nil {
		log.Fatalf("Error: FindInstance: NewInstancesPager returns %v", err)
		return nil, err
	}

	instances, err = pager.GetAllWithContext(ctx)
	if err != nil {
		return nil, err
	}
	log.Debugf("FindInstance: len(instances) = %d", len(instances))

	for _, instance = range instances {
		if !strings.HasPrefix(*instance.Name, name) {
			log.Debugf("FindInstance: SKIP %s %s", *instance.Name, *instance.HealthState)
			continue
		}

		log.Debugf("FindInstance: FOUND %s %s", *instance.Name, *instance.HealthState)

		getInstanceOptions := vpcSvc.NewGetInstanceOptions(*instance.ID)

		foundInstance, _, err := vpcSvc.GetInstanceWithContext(ctx, getInstanceOptions)
		if err != nil {
			return nil, err
		} else {
			return foundInstance, nil
		}
	}

	return nil, nil
}

func (vpc *Vpc) CreateInstance(instancePrototype *vpcv1.InstancePrototypeInstanceByImage) (*vpcv1.Instance, error) {
	var (
		vpcSvc   *vpcv1.VpcV1
		ctx      context.Context
		cancel   context.CancelFunc
		options  *vpcv1.CreateInstanceOptions
		instance *vpcv1.Instance
		err      error
	)

	if vpc.innerVpc == nil {
		return nil, fmt.Errorf("createInstance: VPC not found!")
	}

	vpcSvc = vpc.services.GetVpcSvc()

	ctx, cancel = vpc.services.GetContextWithTimeout()
	defer cancel()

	options = vpcSvc.NewCreateInstanceOptions(instancePrototype)

	instance, _, err = vpcSvc.CreateInstanceWithContext(ctx, options)
	log.Debugf("instance = %+v", instance)
	log.Debugf("err = %+v", err)

	return instance, err
}

func (vpc *Vpc) GetRegionZones() ([]string, error) {
	var (
		vpcSvc         *vpcv1.VpcV1
		ctx            context.Context
		cancel         context.CancelFunc
		vpcRegion      string
		options        *vpcv1.ListRegionZonesOptions
		zoneCollection *vpcv1.ZoneCollection
		result         = make([]string, 0)
		err            error
	)

	if vpc.innerVpc == nil {
		return nil, fmt.Errorf("GetRegionZones: VPC not found!")
	}

	vpcSvc = vpc.services.GetVpcSvc()

	ctx, cancel = vpc.services.GetContextWithTimeout()
	defer cancel()

	vpcRegion, err = vpc.services.GetMetadata().GetVPCRegion()
	if err != nil {
		return nil, err
	}

	options = vpcSvc.NewListRegionZonesOptions(vpcRegion)

	zoneCollection, _, err = vpcSvc.ListRegionZonesWithContext(ctx, options)
	if err != nil {
		return nil, err
	}

	for _, zone := range zoneCollection.Zones {
		log.Debugf("GetRegionZones: FOUND %s %s", *zone.Name, *zone.Status)
		result = append(result, *zone.Name)
	}

	return result, nil
}

func (vpc *Vpc) ListImages() ([]vpcv1.Image, error) {
	var (
		vpcSvc *vpcv1.VpcV1
		ctx    context.Context
		cancel context.CancelFunc
		pager  *vpcv1.ImagesPager
		images []vpcv1.Image
		image  vpcv1.Image
		result = make([]vpcv1.Image, 0)
		err    error
	)

	if vpc.innerVpc == nil {
		return nil, fmt.Errorf("ListImages VPC not found!")
	}

	vpcSvc = vpc.services.GetVpcSvc()

	ctx, cancel = vpc.services.GetContextWithTimeout()
	defer cancel()

	pager, err = vpcSvc.NewImagesPager(&vpcv1.ListImagesOptions{
		Status: []string{
			vpcv1.ListImagesOptionsStatusAvailableConst,
		},
		Visibility: ptr.To(vpcv1.ListImagesOptionsVisibilityPublicConst),
	})
	if err != nil {
		log.Fatalf("Error: ListImages: NewImagesPager returns %v", err)
		return nil, err
	}

	images, err = pager.GetAllWithContext(ctx)
	if err != nil {
		return nil, err
	}

	for _, image = range images {
		if *image.Status == "available" {
			log.Debugf("ListImages: FOUND   %s %s", *image.Name, *image.Status)
			result = append(result, image)
		} else {
			log.Debugf("ListImages: UNKNOWN %s %s", *image.Name, *image.Status)
		}
	}

	return result, nil
}

func (vpc *Vpc) ListSshKeys() ([]vpcv1.Key, error) {
	var (
		vpcSvc *vpcv1.VpcV1
		ctx    context.Context
		cancel context.CancelFunc
		pager  *vpcv1.KeysPager
		keys   []vpcv1.Key
		key    vpcv1.Key
		result = make([]vpcv1.Key, 0)
		err    error
	)

	if vpc.innerVpc == nil {
		return nil, fmt.Errorf("ListSshKeys VPC not found!")
	}

	vpcSvc = vpc.services.GetVpcSvc()

	ctx, cancel = vpc.services.GetContextWithTimeout()
	defer cancel()

	pager, err = vpcSvc.NewKeysPager(&vpcv1.ListKeysOptions{})
	if err != nil {
		log.Fatalf("Error: ListSshKeys: NewKeysPager returns %v", err)
		return nil, err
	}

	keys, err = pager.GetAllWithContext(ctx)
	if err != nil {
		return nil, err
	}

	for _, key = range keys {
		log.Debugf("findKey: FOUND %s", *key.Name)
		result = append(result, key)
	}

	return result, nil
}

func (vpc *Vpc) ListFips() ([]vpcv1.FloatingIP, error) {
	var (
		vpcSvc  *vpcv1.VpcV1
		ctx     context.Context
		cancel  context.CancelFunc
		options *vpcv1.ListFloatingIpsOptions
		pager   *vpcv1.FloatingIpsPager
		fips    []vpcv1.FloatingIP
		fip     vpcv1.FloatingIP
		result  = make([]vpcv1.FloatingIP, 0)
		err     error
	)

	if vpc.innerVpc == nil {
		return nil, fmt.Errorf("ListFips VPC not found!")
	}

	vpcSvc = vpc.services.GetVpcSvc()

	ctx, cancel = vpc.services.GetContextWithTimeout()
	defer cancel()

	options = vpcSvc.NewListFloatingIpsOptions()
	options.SetResourceGroupID(vpc.services.GetResourceGroupID())

	pager, err = vpcSvc.NewFloatingIpsPager(options)
	if err != nil {
		return nil, err
	}

	fips, err = pager.GetAllWithContext(ctx)
	if err != nil {
		return nil, err
	}

	for _, fip = range fips {
		log.Debugf("ListFips: FOUND %s", *fip.Name)
		result = append(result, fip)
	}

	return result, nil
}

func (vpc *Vpc) CreateFIP(instance *vpcv1.Instance) error {
	var (
		vpcSvc                  *vpcv1.VpcV1
		ctx                     context.Context
		cancel                  context.CancelFunc
		createFloatingIPOptions *vpcv1.CreateFloatingIPOptions
		fipName                 string
		zone                    string
		resourceGroupID         string
		fips                    []vpcv1.FloatingIP
		fip                     vpcv1.FloatingIP
		foundFip                *vpcv1.FloatingIP
		listNifaceFipOptions    *vpcv1.ListInstanceNetworkInterfaceFloatingIpsOptions
		fipCollection           *vpcv1.FloatingIPUnpaginatedCollection
		addInstanceOptions      *vpcv1.AddInstanceNetworkInterfaceFloatingIPOptions
		err                     error
	)

	if vpc.innerVpc == nil {
		return fmt.Errorf("CreateFIP VPC not found!")
	}

	if instance == nil {
		return fmt.Errorf("CreateFIP instance is nil!")
	}

	vpcSvc = vpc.services.GetVpcSvc()

	ctx, cancel = vpc.services.GetContextWithTimeout()
	defer cancel()

	fips, err = vpc.ListFips()
	if err != nil {
		return err
	}

	fipName = fmt.Sprintf("%s-fip", *instance.Name)

	foundFip = nil
	for _, fip = range fips {
		if *fip.Name == fipName {
			log.Debugf("CreateFIP: FOUND %s", *fip.Name)
			foundFip = &fip
			break
		}
	}

	if foundFip == nil {
		fmt.Println("Creating floating IP")

		zones, err := vpc.GetRegionZones()
		if err != nil {
			return err
		}
		log.Debugf("CreateFIP: zones = %+v", zones)
		zone = zones[0]

		log.Debugf("CreateFIP: fipName = %s", fipName)
		log.Debugf("CreateFIP: zone    = %s", zone)

		resourceGroupID = vpc.services.GetResourceGroupID()

		createFloatingIPOptions = vpcSvc.NewCreateFloatingIPOptions(&vpcv1.FloatingIPPrototypeFloatingIPByZone{
			Name: &fipName,
			ResourceGroup: &vpcv1.ResourceGroupIdentity{
				ID: &resourceGroupID,
			},
			Zone: &vpcv1.ZoneIdentityByName{
				Name: &zone,
			},
		})

		foundFip, _, err := vpcSvc.CreateFloatingIPWithContext(ctx, createFloatingIPOptions)
		log.Debugf("CreateFIP: foundFip = %+v", foundFip)
		log.Debugf("CreateFIP: err = %+v", err)
		if err != nil {
			return err
		}
	} else {
		fmt.Printf("Found floating IP %s\n", *foundFip.Name)
	}

	for _, niface := range instance.NetworkInterfaces {
		log.Debugf("CreateFIP: niface.ID                = %+v", *niface.ID)
		log.Debugf("CreateFIP: niface.Name              = %+v", *niface.Name)
		log.Debugf("CreateFIP: niface.PrimaryIP.Address = %+v", *niface.PrimaryIP.Address)
		log.Debugf("CreateFIP: niface.PrimaryIP.ID      = %+v", *niface.PrimaryIP.ID)
		log.Debugf("CreateFIP: niface.PrimaryIP.name    = %+v", *niface.PrimaryIP.Name)
		log.Debugf("CreateFIP: niface.PrimaryIP         = %+v", niface.PrimaryIP)
		log.Debugf("CreateFIP: niface                   = %+v", niface)
	}

	listNifaceFipOptions = vpcSvc.NewListInstanceNetworkInterfaceFloatingIpsOptions(
		*instance.ID,
		*instance.NetworkInterfaces[0].ID,
	)

	fipCollection, _, err = vpcSvc.ListInstanceNetworkInterfaceFloatingIpsWithContext(ctx, listNifaceFipOptions)
	log.Debugf("CreateFIP: len(fipCollection).FloatingIps = %d", len(fipCollection.FloatingIps))
	log.Debugf("CreateFIP: err = %+v", err)
	if err != nil {
		return err
	}

	for _, fipElm := range fipCollection.FloatingIps {
		log.Debugf("CreateFIP: fipElm.ID   = %+v", *fipElm.ID)
		log.Debugf("CreateFIP: fipElm.Name = %+v", *fipElm.Name)
		log.Debugf("CreateFIP: fipElm      = %+v", fipElm)

		if *fipElm.Name == fipName {
			log.Debugf("CreateFIP: FOUND existing FIP (%s)", *fipElm.Name)
			fmt.Printf("Floating IP (%s) is already attached to the instance\n", *fipElm.Address)
			return nil
		}
	}

	addInstanceOptions = vpcSvc.NewAddInstanceNetworkInterfaceFloatingIPOptions(
		*instance.ID,
		*instance.NetworkInterfaces[0].ID,
		*fip.ID)

	foundFip, _, err = vpcSvc.AddInstanceNetworkInterfaceFloatingIPWithContext(ctx, addInstanceOptions)
	log.Debugf("CreateFIP: foundFip = %+v", foundFip)
	log.Debugf("CreateFIP: err = %+v", err)
	if err != nil {
		return err
	}

	fmt.Printf("Attaching floating IP (%s) to the instance\n", *foundFip.Address)

	return err
}

func (vpc *Vpc) ID() (string, error) {
	if vpc.innerVpc == nil {
		return "", fmt.Errorf("VPC not found")
	}

	return *vpc.innerVpc.ID, nil
}

func (vpc *Vpc) CRN() (string, error) {
	if vpc.innerVpc == nil || vpc.innerVpc.CRN == nil {
		return "(error)", nil
	}

	return *vpc.innerVpc.CRN, nil
}

func (vpc *Vpc) Name() (string, error) {
	if vpc.innerVpc == nil || vpc.innerVpc.Name == nil {
		return "(error)", nil
	}

	return *vpc.innerVpc.Name, nil
}

func (vpc *Vpc) ObjectName() (string, error) {
	return vpcObjectName, nil
}

func (vpc *Vpc) Run() error {
	// Nothing to do here!
	return nil
}

func (vpc *Vpc) CiStatus(shouldClean bool) {
}

func (vpc *Vpc) ClusterStatus() {
	var (
		isOk         bool
		subnets      []*vpcv1.Subnet
		subnet       *vpcv1.Subnet
		countSubnets int
		err          error
	)

	if vpc.innerVpc == nil {
		fmt.Printf("%s is NOTOK. Could not find a VPC named %s\n", vpcObjectName, vpc.name)
		return
	}

	isOk = true

	switch *vpc.innerVpc.HealthState {
	case "ok":
		//	case "degraded":
		//	case "faulted":
		//	case "inapplicable":
		//	case "failed":
		//	case "deleting":
	default:
		fmt.Printf("%s %s is NOTOK.  The health state is not ok but %s\n", vpcObjectName, vpc.name, *vpc.innerVpc.HealthState)
		isOk = false
	}

	subnets, err = vpc.ListSubnets()
	if err != nil {
		fmt.Printf("%s %s is NOTOK.  Received %v querying subnets\n", vpcObjectName, vpc.name, err)
		isOk = false
	}

	countSubnets = 0
	for _, subnet = range subnets {
		countSubnets++

		if *subnet.Status == "available" {
			fmt.Printf("%s %s found subnet %s\n", vpcObjectName, vpc.name, *subnet.Name)
		} else {
			fmt.Printf("%s %s is NOTOK subnet %s has status %s\n", vpcObjectName, vpc.name, *subnet.Name, *subnet.Status)
			isOk = false
		}
	}
	if countSubnets < 3 {
		fmt.Printf("%s %s is NOTOK expecting at leasst 3 subnets, found %d\n", vpcObjectName, vpc.name, countSubnets)
		isOk = false
	}

	idRules, _ := vpc.FindSecurityGroupsByVPC()
	log.Debugf("idRules = %+v", idRules)

	// func (vpc *VpcV1) ListSecurityGroupsWithContext(ctx context.Context, listSecurityGroupsOptions *ListSecurityGroupsOptions) (result *SecurityGroupCollection, response *core.DetailedResponse, err error) {
	// func (vpc *VpcV1) GetSecurityGroupWithContext(ctx context.Context, getSecurityGroupOptions *GetSecurityGroupOptions) (result *SecurityGroup, response *core.DetailedResponse, err error) {
	// func (vpc *VpcV1) ListSecurityGroupRulesWithContext(ctx context.Context, listSecurityGroupRulesOptions *ListSecurityGroupRulesOptions) (result *SecurityGroupRuleCollection, response *core.DetailedResponse, err error) {

	if isOk {
		fmt.Printf("%s %s is OK.\n", vpcObjectName, vpc.name)
	}
}

func (vpc *Vpc) Priority() (int, error) {
	return 100, nil
}
