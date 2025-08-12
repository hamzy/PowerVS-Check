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
	"regexp"
	"strings"

	"github.com/IBM/go-sdk-core/v5/core"
	"github.com/IBM/vpc-go-sdk/vpcv1"
)

type LoadBalancer struct {
	name string

	services *Services

	innerLb *vpcv1.LoadBalancer
}

const (
	lbObjectName = "Load Balancer"
)

func NewLoadBalancer(services *Services) ([]RunnableObject, []error) {
	var (
		lbName   string
		vpcSvc   *vpcv1.VpcV1
		ctx      context.Context
		cancel   context.CancelFunc
		lbIds    []string
		ros      []RunnableObject
		lbs      []*LoadBalancer
		response *core.DetailedResponse
		err      error
		errs     []error
	)

	lbName, err = services.GetMetadata().GetObjectName(RunnableObject(&LoadBalancer{}))
	if err != nil {
		return []RunnableObject{&LoadBalancer{
			name:     lbName,
			services: services,
			innerLb:  nil,
		}}, []error{err}
	}

	vpcSvc = services.GetVpcSvc()

	ctx, cancel = services.GetContextWithTimeout()
	defer cancel()

	if fUseTagSearch {
		lbIds, err = listByTag(TagTypeLoadBalancer, services)
	} else {
		lbIds, err = listLoadBalancersByName(vpcSvc, ctx, lbName)
	}
	if err != nil {
		return []RunnableObject{&LoadBalancer{
			name:     lbName,
			services: services,
			innerLb:  nil,
		}}, []error{err}
	}
	log.Debugf("NewLoadBalancer: lbIds = %+v", lbIds)

	lbs = []*LoadBalancer{
		{
			name:     "(internal load balancer)",
			services: services,
			innerLb:  nil,
		},
		{
			name:     "(external load balancer)",
			services: services,
			innerLb:  nil,
		},
		{
			name:     "(kube load balancer)",
			services: services,
			innerLb:  nil,
		},
	}
	ros = make([]RunnableObject, 3)
	errs = make([]error, 3)

	for _, lbId := range lbIds {
		var (
			options *vpcv1.GetLoadBalancerOptions
			innerLb *vpcv1.LoadBalancer
		)

		log.Debugf("NewLoadBalancer: lbId = %s", lbId)

		options = vpcSvc.NewGetLoadBalancerOptions(lbId)

		innerLb, response, err = vpcSvc.GetLoadBalancerWithContext(ctx, options)
		if err != nil {
			log.Fatalf("NewLoadBalancer could not GetLoadBalancerWithContext(%s): %s", lbId, response)
			continue
		} else if innerLb == nil {
			log.Fatalf("NewLoadBalancer nil return from GetLoadBalancerWithContext(%s)", lbId)
			continue
		}

		switch loadBalancerType(*innerLb.Name) {
		case LoadBalancerTypeUnknown:
			log.Fatalf("NewLoadBalancer could not GetLoadBalancerWithContext(%s): %s", lbId, response)
		case LoadBalancerTypeInternal:
			lbs[0].name = *innerLb.Name
			lbs[0].innerLb = innerLb
			errs[0] = err
		case LoadBalancerTypeExternal:
			lbs[1].name = *innerLb.Name
			lbs[1].innerLb = innerLb
			errs[1] = err
		case LoadBalancerTypeKube:
			lbs[2].name = *innerLb.Name
			lbs[2].innerLb = innerLb
			errs[2] = err
		}
	}

	// Go does not support type converting the entire array.
	// So we do it manually.
	for i, v := range lbs {
		ros[i] = RunnableObject(v)
	}

	return ros, errs
}

type LoadBalancerType int

const (
	LoadBalancerTypeUnknown LoadBalancerType = iota
	LoadBalancerTypeInternal
	LoadBalancerTypeExternal
	LoadBalancerTypeKube
)

func loadBalancerType(name string) LoadBalancerType {
	if regexp.MustCompile("loadbalancer-int$").MatchString(name) {
		return LoadBalancerTypeInternal
	} else if regexp.MustCompile("loadbalancer$").MatchString(name) {
		return LoadBalancerTypeExternal
	} else if regexp.MustCompile("^kube-").MatchString(name) {
		return LoadBalancerTypeKube
	} else {
		return LoadBalancerTypeUnknown
	}
}

// listLoadBalancersByName list the load balancers matching by name in the IBM Cloud.
func listLoadBalancersByName(vpcSvc *vpcv1.VpcV1, ctx context.Context, infraID string) ([]string, error) {
	var (
		options      *vpcv1.ListLoadBalancersOptions
		lbCollection *vpcv1.LoadBalancerCollection
		response     *core.DetailedResponse
		foundOne     = false
		result       = make([]string, 0, 3)
		lb           vpcv1.LoadBalancer
		err          error
	)

	log.Debugf("Listing load balancers by NAME")

	select {
	case <-ctx.Done():
		log.Debugf("listLoadBalancersByName: case <-ctx.Done()")
		return nil, ctx.Err() // we're cancelled, abort
	default:
	}

	options = vpcSvc.NewListLoadBalancersOptions()
	// @WHY options.SetResourceGroupID(resourceGroupID)

	lbCollection, response, err = vpcSvc.ListLoadBalancersWithContext(ctx, options)
	if err != nil {
		return nil, fmt.Errorf("failed to list load balancers: err = %w, response = %v", err, response)
	}

	for _, lb = range lbCollection.LoadBalancers {
		select {
		case <-ctx.Done():
			log.Debugf("listLoadBalancersByName: case <-ctx.Done()")
			return nil, ctx.Err() // we're cancelled, abort
		default:
		}

		if strings.Contains(*lb.Name, infraID) {
			foundOne = true
			log.Debugf("listLoadBalancersByName: FOUND: %s, %s, %s", *lb.ID, *lb.Name, *lb.ProvisioningStatus)
			result = append(result, *lb.ID)
		}
	}
	if !foundOne {
		log.Debugf("listLoadBalancersByName: NO matching loadbalancers against: %s", infraID)

		for _, loadbalancer := range lbCollection.LoadBalancers {
			select {
			case <-ctx.Done():
				log.Debugf("listLoadBalancersByName: case <-ctx.Done()")
				return nil, ctx.Err() // we're cancelled, abort
			default:
			}

			log.Debugf("listLoadBalancersByName: loadbalancer: %s", *loadbalancer.Name)
		}
	}

	return result, nil
}

func (lb *LoadBalancer) listLoadBalancerPools() ([]*vpcv1.LoadBalancerPool, error) {
	var (
		ctx            context.Context
		cancel         context.CancelFunc
		vpcSvc         *vpcv1.VpcV1
		lbPoolsOptions *vpcv1.ListLoadBalancerPoolsOptions
		lbpc           *vpcv1.LoadBalancerPoolCollection
		result         []*vpcv1.LoadBalancerPool
		err            error
	)

	ctx, cancel = lb.services.GetContextWithTimeout()
	defer cancel()

	select {
	case <-ctx.Done():
		log.Debugf("listLoadBalancerPools: case <-ctx.Done()")
		return nil, ctx.Err() // we're cancelled, abort
	default:
	}

	vpcSvc = lb.services.GetVpcSvc()

	lbPoolsOptions = vpcSvc.NewListLoadBalancerPoolsOptions(*lb.innerLb.ID)

	lbpc, _, err = vpcSvc.ListLoadBalancerPoolsWithContext(ctx, lbPoolsOptions)
	if err != nil {
		fmt.Printf("%s %s could not get pools: %v\n", lbObjectName, lb.name, err)
		return nil, err
	}

	result = make([]*vpcv1.LoadBalancerPool, 0)

	for _, lbp := range lbpc.Pools {
		select {
		case <-ctx.Done():
			log.Debugf("listLoadBalancerPools: case <-ctx.Done()")
			return nil, ctx.Err() // we're cancelled, abort
		default:
		}

		log.Debugf("listLoadBalancerPools: FOUND %s", *lbp.Name)
		result = append(result, &lbp)
	}

	return result, nil
}

func (lb *LoadBalancer) listLoadBalancerPoolMembers(id string) ([]*vpcv1.LoadBalancerPoolMember, error) {
	var (
		ctx         context.Context
		cancel      context.CancelFunc
		vpcSvc      *vpcv1.VpcV1
		llpmOptions *vpcv1.ListLoadBalancerPoolMembersOptions
		lbpmc       *vpcv1.LoadBalancerPoolMemberCollection
		result      []*vpcv1.LoadBalancerPoolMember
		err         error
	)

	ctx, cancel = lb.services.GetContextWithTimeout()
	defer cancel()

	select {
	case <-ctx.Done():
		log.Debugf("listLoadBalancerPoolMembers: case <-ctx.Done()")
		return nil, ctx.Err() // we're cancelled, abort
	default:
	}

	vpcSvc = lb.services.GetVpcSvc()

	llpmOptions = vpcSvc.NewListLoadBalancerPoolMembersOptions(*lb.innerLb.ID, id)

	lbpmc, _, err = vpcSvc.ListLoadBalancerPoolMembersWithContext(ctx, llpmOptions)
	if err != nil {
		fmt.Printf("%s %s could not find pool members for %s: %v\n", lbObjectName, lb.name, id, err)
		return nil, err
	}

	result = make([]*vpcv1.LoadBalancerPoolMember, 0)

	for _, lbpm := range lbpmc.Members {
		select {
		case <-ctx.Done():
			log.Debugf("listLoadBalancerPoolMembers: case <-ctx.Done()")
			return nil, ctx.Err() // we're cancelled, abort
		default:
		}

		log.Debugf("listLoadBalancerPoolMembers: lbpm = %+v", lbpm)
		result = append(result, &lbpm)
	}

	return result, nil
}

func (lb *LoadBalancer) checkLoadBalancerPool(poolNames []string, poolUserName string) bool {
	var (
		ctx           context.Context
		cancel        context.CancelFunc
		vpcSvc        *vpcv1.VpcV1
		lbps          []*vpcv1.LoadBalancerPool
		lbpGetOptions *vpcv1.GetLoadBalancerPoolOptions
		lbp           *vpcv1.LoadBalancerPool
		lbpms         []*vpcv1.LoadBalancerPoolMember
		err           error
	)

	if lb.innerLb == nil {
		return false
	}

	ctx, cancel = lb.services.GetContextWithTimeout()
	defer cancel()

	select {
	case <-ctx.Done():
		log.Debugf("checkLoadBalancerPool: case <-ctx.Done()")
		return false
	default:
	}

	vpcSvc = lb.services.GetVpcSvc()

	lbps, err = lb.listLoadBalancerPools()
	if err != nil {
		return false
	}
	log.Debugf("checkLoadBalancerPool: lbps = %+v", lbps)

	for _, lbpElm := range lbps {
		select {
		case <-ctx.Done():
			log.Debugf("checkLoadBalancerPool: case <-ctx.Done()")
			return false
		default:
		}

		found := false
		for _, poolName := range poolNames {
			if strings.Contains(*lbpElm.Name, poolName) {
				found = true
			}
		}
		if !found {
			continue
		}

		lbpGetOptions = vpcSvc.NewGetLoadBalancerPoolOptions(*lb.innerLb.ID, *lbpElm.ID)
		log.Debugf("checkLoadBalancerPool: lbpGetOptions = %s %s", *lbpGetOptions.LoadBalancerID, *lbpGetOptions.ID)

		lbp, _, err = vpcSvc.GetLoadBalancerPoolWithContext(ctx, lbpGetOptions)
		if err != nil {
			fmt.Printf("%s %s could not get load balancer pool: %v\n", lbObjectName, lb.name, err)
			return false
		}
		log.Debugf("checkLoadBalancerPool: lbp = %+v", lbp)
		break
	}

	if lbp == nil {
		fmt.Printf("%s %s could not find pool %s.\n", lbObjectName, lb.name, poolUserName)
		return false
	}

	lbpms, err = lb.listLoadBalancerPoolMembers(*lbp.ID)
	if err != nil {
		return false
	}

	okHealthCount := 0
	for _, lbpm := range lbpms {
		select {
		case <-ctx.Done():
			log.Debugf("checkLoadBalancerPool: case <-ctx.Done()")
			return false
		default:
		}

		log.Debugf("checkLoadBalancerPool: lbpm = %+v", lbpm)

		if *lbpm.Health == "ok" {
			okHealthCount += 1
		}
	}
	if okHealthCount == 0 {
		fmt.Printf("%s %s did not find a healthy member of pool %s.\n", lbObjectName, lb.name, poolUserName)
		return false
	} else {
		fmt.Printf("%s %s found %d healthy members of pool %s.\n", lbObjectName, lb.name, okHealthCount, poolUserName)
		return true
	}
}

func (lb *LoadBalancer) CRN() (string, error) {
	if lb.innerLb == nil || lb.innerLb.CRN == nil {
		return "(error)", nil
	}

	return *lb.innerLb.CRN, nil
}

func (lb *LoadBalancer) Name() (string, error) {
	if lb.innerLb == nil || lb.innerLb.Name == nil {
		return "(error)", nil
	}

	return *lb.innerLb.Name, nil
}

func (lb *LoadBalancer) ObjectName() (string, error) {
	return lbObjectName, nil
}

func (lb *LoadBalancer) Run() error {
	if lb.innerLb != nil {
		lb.name = *lb.innerLb.Name
	}

	return nil
}

func (lb *LoadBalancer) CiStatus(shouldClean bool) {
}

func (lb *LoadBalancer) ClusterStatus() {
	if lb.innerLb == nil {
		fmt.Printf("%s is NOTOK. Could not find a LB named %s\n", lbObjectName, lb.name)
		return
	}

	if *lb.innerLb.OperatingStatus != "online" {
		fmt.Printf("%s %s is NOTOK. The status is %s\n", lbObjectName, lb.name, *lb.innerLb.OperatingStatus)
		return
	}

	switch loadBalancerType(*lb.innerLb.Name) {
	case LoadBalancerTypeUnknown:
	case LoadBalancerTypeInternal:
		// Internal Load Balancer
		if !lb.checkLoadBalancerPool([]string{
			"pool-6443",
		},
			"port 6443") {
			fmt.Printf("%s %s is NOTOK.\n", lbObjectName, lb.name)
			return
		}

		if !lb.checkLoadBalancerPool([]string{
			"machine-config-server",
			"additional-pool-22623",
		},
			"machine config server") {
			fmt.Printf("%s %s is NOTOK.\n", lbObjectName, lb.name)
			return
		}
	case LoadBalancerTypeExternal:
		// External Load Balancer
		if !lb.checkLoadBalancerPool([]string{
			"pool-6443",
		},
			"port 6443") {
			fmt.Printf("%s %s is NOTOK.\n", lbObjectName, lb.name)
			return
		}
	case LoadBalancerTypeKube:
		// The Kube pool
		if !lb.checkLoadBalancerPool([]string{
			"tcp-80",
		},
			"port 80") {
			fmt.Printf("%s %s is NOTOK.\n", lbObjectName, lb.name)
			return
		}

		if !lb.checkLoadBalancerPool([]string{
			"tcp-443",
		},
			"port 443") {
			fmt.Printf("%s %s is NOTOK.\n", lbObjectName, lb.name)
			return
		}
	}

	fmt.Printf("%s %s is OK.\n", lbObjectName, lb.name)
}

func (lb *LoadBalancer) Priority() (int, error) {
	return 70, nil
}
