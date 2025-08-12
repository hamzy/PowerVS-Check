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

	"github.com/IBM/go-sdk-core/v5/core"

	// https://raw.githubusercontent.com/IBM/networking-go-sdk/refs/heads/master/dnsrecordsv1/dns_records_v1.go
	"github.com/IBM/networking-go-sdk/dnsrecordsv1"
	//
	"github.com/IBM/networking-go-sdk/dnssvcsv1"
	// https://raw.githubusercontent.com/IBM/networking-go-sdk/refs/heads/master/zonesv1/zones_v1.go
	"github.com/IBM/networking-go-sdk/zonesv1"

	"github.com/IBM/platform-services-go-sdk/resourcecontrollerv2"
)

type DNS struct {
	//
	services      *Services

	//
	dnsSvc        *dnssvcsv1.DnsSvcsV1

	//
	dnsRecordsSvc *dnsrecordsv1.DnsRecordsV1
}

const (
	dnsObjectName = "Domain Name Service"
)

func NewDNS(services *Services) ([]RunnableObject, []error) {
	var (
		dnsSvc        *dnssvcsv1.DnsSvcsV1
		dnsRecordsSvc *dnsrecordsv1.DnsRecordsV1
		err           error
	)

	dnsSvc, dnsRecordsSvc, err = initDNSService(services)
	if err != nil {
		return []RunnableObject{}, []error{ err }
	}

	return []RunnableObject{&DNS{
		services:      services,
		dnsSvc:        dnsSvc,
		dnsRecordsSvc: dnsRecordsSvc,
	}}, []error{ nil }
}

func initDNSService(services *Services) (*dnssvcsv1.DnsSvcsV1, *dnsrecordsv1.DnsRecordsV1, error) {
	var (
		authenticator       core.Authenticator
		dnsService          *dnssvcsv1.DnsSvcsV1
		globalOptions       *dnsrecordsv1.DnsRecordsV1Options
		controllerSvc       *resourcecontrollerv2.ResourceControllerV2
		metadata            *Metadata
		listResourceOptions *resourcecontrollerv2.ListResourceInstancesOptions
		dnsRecordService    *dnsrecordsv1.DnsRecordsV1
		zonesService        *zonesv1.ZonesV1
		listZonesOptions    *zonesv1.ListZonesOptions
		listZonesResponse   *zonesv1.ListZonesResp
		zoneID              string
		err                 error
	)

	authenticator = &core.IamAuthenticator{
		ApiKey: services.GetApiKey(),
	}
	err = authenticator.Validate()
	if err != nil {
		return nil, nil, err
	}

	dnsService, err = dnssvcsv1.NewDnsSvcsV1(&dnssvcsv1.DnsSvcsV1Options{
		Authenticator: authenticator,
	})
	if err != nil {
		return nil, nil, err
	}

	authenticator = &core.IamAuthenticator{
		ApiKey: services.GetApiKey(),
	}
	err = authenticator.Validate()
	if err != nil {
		return nil, nil, err
	}

	controllerSvc = services.GetControllerSvc()
	metadata = services.GetMetadata()

	listResourceOptions = controllerSvc.NewListResourceInstancesOptions()
	listResourceOptions.SetResourceID("75874a60-cb12-11e7-948e-37ac098eb1b9") // CIS service ID

	listResourceInstancesResponse, _, err := controllerSvc.ListResourceInstances(listResourceOptions)
	if err != nil {
		return nil, nil, err
	}

	for _, instance := range listResourceInstancesResponse.Resources {
		log.Debugf("initDNSService: instance.CRN = %s", *instance.CRN)

		authenticator = &core.IamAuthenticator{
			ApiKey: services.GetApiKey(),
		}
		err = authenticator.Validate()
		if err != nil {
			return nil, nil, err
		}

		zonesService, err = zonesv1.NewZonesV1(&zonesv1.ZonesV1Options{
			Authenticator: authenticator,
			Crn:           instance.CRN,
		})
		if err != nil {
			return nil, nil, err
		}
		log.Debugf("initDNSService: zonesService = %+v", zonesService)

		listZonesOptions = zonesService.NewListZonesOptions()

		listZonesResponse, _, err = zonesService.ListZones(listZonesOptions)
		if err != nil {
			return nil, nil, err
		}

		for _, zone := range listZonesResponse.Result {
			log.Debugf("initDNSService: zone.Name = %s", *zone.Name)
			log.Debugf("initDNSService: zone.ID   = %s", *zone.ID)

			if *zone.Name == metadata.GetBaseDomain() {
				zoneID = *zone.ID
			}
		}
	}
	log.Debugf("initDNSService: zoneID = %s", zoneID)

	authenticator = &core.IamAuthenticator{
		ApiKey: services.GetApiKey(),
	}
	err = authenticator.Validate()
	if err != nil {
		return nil, nil, err
	}

	CRN := metadata.GetCISInstanceCRN()

	globalOptions = &dnsrecordsv1.DnsRecordsV1Options{
		Authenticator:  authenticator,
		Crn:            &CRN,
		ZoneIdentifier: &zoneID,
	}
	dnsRecordService, err = dnsrecordsv1.NewDnsRecordsV1(globalOptions)
	log.Debugf("initDNSService: dnsRecordService = %+v", dnsRecordService)

	return dnsService, dnsRecordService, err
}

// listDNSRecords lists DNS records for the cluster.
func (dns *DNS) listDNSRecords() ([]string, error) {
	var (
		metadata *Metadata
		ctx      context.Context
		cancel   context.CancelFunc
		result   []string
	)

	log.Debugf("listDNSRecords: Listing DNS records")

	metadata = dns.services.GetMetadata()

	ctx, cancel = dns.services.GetContextWithTimeout()
	defer cancel()

	select {
	case <-ctx.Done():
		log.Debugf("listDNSRecords: case <-ctx.Done()")
		return nil, ctx.Err() // we're cancelled, abort
	default:
	}

	var (
		foundOne       = false
		perPage  int64 = 20
		page     int64 = 1
		moreData       = true
	)

	dnsRecordsOptions := dns.dnsRecordsSvc.NewListAllDnsRecordsOptions()
	dnsRecordsOptions.PerPage = &perPage
	dnsRecordsOptions.Page = &page

	result = make([]string, 0, 3)

	dnsMatcher, err := regexp.Compile(fmt.Sprintf(`.*\Q%s.%s\E$`, metadata.GetClusterName(), metadata.GetBaseDomain()))
	if err != nil {
		return nil, fmt.Errorf("failed to build DNS records matcher: %w", err)
	}

	for moreData {
		select {
		case <-ctx.Done():
			log.Debugf("listDNSRecords: case <-ctx.Done()")
			return nil, ctx.Err() // we're cancelled, abort
		default:
		}

		dnsResources, detailedResponse, err := dns.dnsRecordsSvc.ListAllDnsRecordsWithContext(ctx, dnsRecordsOptions)
		if err != nil {
			return nil, fmt.Errorf("failed to list DNS records: %w and the response is: %s", err, detailedResponse)
		}

		for _, record := range dnsResources.Result {
			// Match all of the cluster's DNS records
			nameMatches := dnsMatcher.Match([]byte(*record.Name))
			contentMatches := dnsMatcher.Match([]byte(*record.Content))
			if nameMatches || contentMatches {
				foundOne = true
				log.Debugf("listDNSRecords: FOUND: %v, %v", *record.ID, *record.Name)
				result = append(result, *record.Name)
			}
		}

		log.Debugf("listDNSRecords: PerPage = %v, Page = %v, Count = %v", *dnsResources.ResultInfo.PerPage, *dnsResources.ResultInfo.Page, *dnsResources.ResultInfo.Count)

		moreData = *dnsResources.ResultInfo.PerPage == *dnsResources.ResultInfo.Count
		log.Debugf("listDNSRecords: moreData = %v", moreData)

		page++
	}
	if !foundOne {
		log.Debugf("listDNSRecords: NO matching DNS against: %s", metadata.GetInfraID())
		for moreData {
			select {
			case <-ctx.Done():
				log.Debugf("listDNSRecords: case <-ctx.Done()")
				return nil, ctx.Err() // we're cancelled, abort
			default:
			}

			dnsResources, detailedResponse, err := dns.dnsRecordsSvc.ListAllDnsRecordsWithContext(ctx, dnsRecordsOptions)
			if err != nil {
				return nil, fmt.Errorf("failed to list DNS records: %w and the response is: %s", err, detailedResponse)
			}
			for _, record := range dnsResources.Result {
				log.Debugf("listDNSRecords: FOUND: DNS: %v, %v", *record.ID, *record.Name)
			}
			moreData = *dnsResources.ResultInfo.PerPage == *dnsResources.ResultInfo.Count
			page++
		}
	}

	return result, nil
}

func (dns *DNS) CRN() (string, error) {
	return dns.services.GetMetadata().GetCISInstanceCRN(), nil
}

func (dns *DNS) Name() (string, error) {
	return "(@TODO not implemented yet)", nil
}

func (dns *DNS) ObjectName() (string, error) {
	return dnsObjectName, nil
}

func (dns *DNS) Run() error {
	// Nothing to do!
	return nil
}

func (dns *DNS) CiStatus(shouldClean bool) {
}

func (dns *DNS) ClusterStatus() {
	var (
		metadata *Metadata
		records  []string
		patterns = []string{ "api-int", "api", "*.apps" }
		name     string
		found    bool
		err      error
	)

	metadata = dns.services.GetMetadata()

	records, err = dns.listDNSRecords()
	if err != nil {
		fmt.Printf("%s is NOTOK. Could not list DNS records: %v\n", dnsObjectName, err)
		return
	}
	log.Debugf("Valid: records = %+v", records)

	if len(records) != 3 {
		fmt.Printf("%s is NOTOK. Expecting 3 DNS records, found %d (%+v)\n", dnsObjectName, len(records), records)
		return
	}

	for _, pattern := range patterns {
		name = fmt.Sprintf("%s.%s.%s", pattern, metadata.GetClusterName(), metadata.GetBaseDomain())
		log.Debugf("Valid: name = %s", name)

		found = false
		for _, record := range records {
			if record == name {
				found = true
			}
		}
		if !found {
			fmt.Printf("%s is NOTOK. Expecting DNS record %s to exist\n", dnsObjectName, name)
			return
		}

		// @TODO maybe do a DNS lookup on the name?
	}

	fmt.Printf("%s is OK.\n", dnsObjectName)
	return
}

func (dns *DNS) Priority() (int, error) {
	return 10, nil
}
