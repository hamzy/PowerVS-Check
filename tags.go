package main

import (
	"context"
	"fmt"

	"github.com/IBM-Cloud/bluemix-go/crn"
	"github.com/IBM/go-sdk-core/v5/core"
	"github.com/IBM/platform-services-go-sdk/globalsearchv2"
	"k8s.io/utils/ptr"
)

// TagType The different states deleting a job can take.
type TagType int

const (
	// TagTypeVPC is for Virtual Private Cloud types.
	TagTypeVPC TagType = iota

	// TagTypeLoadBalancer is for Load Balancer types.
	TagTypeLoadBalancer

	// TagTypeCloudInstance is for Virtual Machine instance types.
	TagTypeCloudInstance

	// TagTypePublicGateway is for Public Gateway types.
	TagTypePublicGateway

	// TagTypeFloatingIP is for Floating IP types.
	TagTypeFloatingIP

	// TagTypeNetworkACL is for Network Acces Control List types.
	TagTypeNetworkACL

	// TagTypeSubnet is for Subnet types.
	TagTypeSubnet

	// TagTypeSecurityGroup is for Security Group types.
	TagTypeSecurityGroup

	// TagTypeTransitGateway is for Transit Gateway types.
	TagTypeTransitGateway

	// TagTypeServiceInstance is for Service Instance types.
	TagTypeServiceInstance

	// TagTypeCloudObjectStorage is for Cloud Object Storage types.
	TagTypeCloudObjectStorage
)

var (
	fUseTagSearch = false
)

// listByTag list IBM Cloud resources by matching tag.
func listByTag(tagType TagType, services *Services) ([]string, error) {
	var (
		clusterName         string
		query               string
		ctx                 context.Context
		cancel              context.CancelFunc
		authenticator       *core.IamAuthenticator
		globalSearchOptions *globalsearchv2.GlobalSearchV2Options
		searchService       *globalsearchv2.GlobalSearchV2
		moreData                  = true
		perPage             int64 = 100
		searchCursor        string
		searchOptions       *globalsearchv2.SearchOptions
		scanResult          *globalsearchv2.ScanResult
		response            *core.DetailedResponse
		crnStruct           crn.CRN
		result              []string
		err                 error
	)

	clusterName = services.GetMetadata().GetClusterName()

	switch tagType {
	case TagTypeVPC:
		query = fmt.Sprintf("tags:%s AND family:is AND type:vpc", clusterName)
	case TagTypeLoadBalancer:
		query = fmt.Sprintf("tags:%s AND family:is AND type:load-balancer", clusterName)
	case TagTypeCloudInstance:
		query = fmt.Sprintf("tags:%s AND family:is AND type:instance", clusterName)
	case TagTypePublicGateway:
		query = fmt.Sprintf("tags:%s AND family:is AND type:public-gateway", clusterName)
	case TagTypeFloatingIP:
		query = fmt.Sprintf("tags:%s AND family:is AND type:floating-ip", clusterName)
	case TagTypeNetworkACL:
		query = fmt.Sprintf("tags:%s AND family:is AND type:network-acl", clusterName)
	case TagTypeSubnet:
		query = fmt.Sprintf("tags:%s AND family:is AND type:subnet", clusterName)
	case TagTypeSecurityGroup:
		query = fmt.Sprintf("tags:%s AND family:is AND type:security-group", clusterName)
	case TagTypeTransitGateway:
		query = fmt.Sprintf("tags:%s AND family:resource_controller AND type:gateway", clusterName)
	case TagTypeServiceInstance:
		query = fmt.Sprintf("tags:%s AND family:resource_controller AND type:resource-instance AND crn:crn\\:v1\\:bluemix\\:public\\:power-iaas*", clusterName)
	case TagTypeCloudObjectStorage:
		query = fmt.Sprintf("tags:%s AND family:resource_controller AND type:resource-instance AND crn:crn\\:v1\\:bluemix\\:public\\:cloud-object-storage*", clusterName)
	default:
		return nil, fmt.Errorf("listByTag: tagType %d is unknown", tagType)
	}
	log.Debugf("listByTag: query = %s", query)

	ctx, cancel = services.GetContextWithTimeout()
	defer cancel()

	authenticator = &core.IamAuthenticator{
		ApiKey: services.GetApiKey(),
	}
	err = authenticator.Validate()
	if err != nil {
		return nil, err
	}

	globalSearchOptions = &globalsearchv2.GlobalSearchV2Options{
		URL:           globalsearchv2.DefaultServiceURL,
		Authenticator: authenticator,
	}

	searchService, err = globalsearchv2.NewGlobalSearchV2(globalSearchOptions)
	if err != nil {
		return nil, fmt.Errorf("listByTag: globalsearchv2.NewGlobalSearchV2: %w", err)
	}

	result = make([]string, 0)

	for moreData {
		searchOptions = &globalsearchv2.SearchOptions{
			Query: &query,
			Limit: ptr.To(perPage),
			// default Fields: []string{"account_id", "name", "type", "family", "crn"},
			// all     Fields: []string{"*"},
		}
		if searchCursor != "" {
			searchOptions.SetSearchCursor(searchCursor)
		}
		log.Debugf("listByTag: searchOptions = %+v", searchOptions)

		scanResult, response, err = searchService.SearchWithContext(ctx, searchOptions)
		if err != nil {
			return nil, fmt.Errorf("listByTag: searchService.SearchWithContext: err = %w, response = %v", err, response)
		}
		if scanResult.SearchCursor != nil {
			log.Debugf("listByTag: scanResult = %+v, scanResult.SearchCursor = %+v, len scanResult.Items = %d", scanResult, *scanResult.SearchCursor, len(scanResult.Items))
		} else {
			log.Debugf("listByTag: scanResult = %+v, scanResult.SearchCursor = nil, len scanResult.Items = %d", scanResult, len(scanResult.Items))
		}

		for _, item := range scanResult.Items {
			crnStruct, err = crn.Parse(*item.CRN)
			if err != nil {
				log.Debugf("listByTag: crn = %s", *item.CRN)
				return nil, fmt.Errorf("listByTag: could not parse CRN property")
			}
			log.Debugf("listByTag: crnStruct = %v, crnStruct.Resource = %v", crnStruct, crnStruct.Resource)

			// Append the ID part of the CRN if it exists
			if crnStruct.Resource == "" {
				result = append(result, *item.CRN)
			} else {
				result = append(result, crnStruct.Resource)
			}
		}

		moreData = int64(len(scanResult.Items)) == perPage
		if moreData {
			if scanResult.SearchCursor != nil {
				searchCursor = *scanResult.SearchCursor
			}
		}
	}

	return result, err
}
