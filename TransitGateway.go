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

	"github.com/IBM/go-sdk-core/v5/core"

	// https://raw.githubusercontent.com/IBM/networking-go-sdk/refs/heads/master/transitgatewayapisv1/transit_gateway_apis_v1.go
	"github.com/IBM/networking-go-sdk/transitgatewayapisv1"
)

type TransitGateway struct {
	name string

	tgClient *transitgatewayapisv1.TransitGatewayApisV1

	services *Services

	innerTg *transitgatewayapisv1.TransitGateway
}

const (
	tgObjectName = "Transit Gateway"
)

func NewTransitGateway(services *Services) ([]RunnableObject, []error) {
	var (
		tgName         string
		tgClient       *transitgatewayapisv1.TransitGatewayApisV1
		ctx            context.Context
		cancel         context.CancelFunc
		foundInstances []string
		tg             *TransitGateway
		tgs            []RunnableObject
		errs           []error
		idxTg          int
		err            error
	)

	tg = &TransitGateway{
		name:     "",
		services: services,
		innerTg:  nil,
	}

	tgName, err = services.GetMetadata().GetObjectName(RunnableObject(&TransitGateway{}))
	if err != nil {
		return []RunnableObject{tg}, []error{err}
	}
	log.Debugf("NewTransitGateway: tgName = %s", tgName)
	if tgName == "" {
		return nil, nil
	}
	tg.name = tgName

	tgClient = services.GetTgClient()

	ctx, cancel = services.GetContextWithTimeout()
	defer cancel()

	if fUseTagSearch {
		foundInstances, err = listByTag(TagTypeTransitGateway, services)
	} else {
		foundInstances, err = findTransitGateway(tgClient, ctx, tgName)
	}
	if err != nil {
		return []RunnableObject{tg}, []error{err}
	}
	log.Debugf("NewTransitGateway: foundInstances = %+v, err = %v", foundInstances, err)

	tgs = make([]RunnableObject, 1)
	errs = make([]error, 1)

	idxTg = 0
	tgs[idxTg] = tg

	if len(foundInstances) == 0 {
		errs[idxTg] = fmt.Errorf("Unable to find %s named %s", tgObjectName, tgName)
	}

	log.Debugf("NewTransitGateway: len(foundInstances) = %d", len(foundInstances))
	for _, instanceID := range foundInstances {
		var (
			getTransitGatewayOptions *transitgatewayapisv1.GetTransitGatewayOptions
			innerTg                  *transitgatewayapisv1.TransitGateway
			innerTgName              string
		)

		getTransitGatewayOptions = tgClient.NewGetTransitGatewayOptions(instanceID)

		innerTg, _, err = tgClient.GetTransitGatewayWithContext(ctx, getTransitGatewayOptions)
		if err != nil {
			// Just in case.
			innerTg = nil
		}

		if innerTg != nil {
			innerTgName = *innerTg.Name
		} else {
			innerTgName = tgName
		}
		tg.name = innerTgName
		tg.innerTg = innerTg

		if idxTg > 0 {
			log.Debugf("NewTransitGateway: appending to tgs")

			tgs = append(tgs, tg)
			errs = append(errs, err)
		} else {
			log.Debugf("NewTransitGateway: altering first tgs")

			tgs[idxTg] = tg
			errs[idxTg] = err
		}

		idxTg++
	}

	return tgs, errs
}

// findTransitGateway find a Transit Gateway matching by name in the IBM Cloud.
func findTransitGateway(tgClient *transitgatewayapisv1.TransitGatewayApisV1, ctx context.Context, name string) ([]string, error) {
	var (
		listTransitGatewaysOptions *transitgatewayapisv1.ListTransitGatewaysOptions
		gatewayCollection          *transitgatewayapisv1.TransitGatewayCollection
		gateway                    transitgatewayapisv1.TransitGateway
		response                   *core.DetailedResponse
		err                        error
		foundOne                         = false
		perPage                    int64 = 32
		moreData                         = true
	)

	log.Debugf("Listing Transit Gateways (%s) by NAME", name)

	matchFunc := func(tg transitgatewayapisv1.TransitGateway, match string) bool {
		if match == "" {
			return false
		}
		if strings.Contains(*tg.Name, match) {
			return true
		}
		if *tg.Crn == match {
			return true
		}
		return false
	}

	select {
	case <-ctx.Done():
		log.Debugf("listTransitGatewaysByName: case <-ctx.Done()")
		return nil, ctx.Err() // we're cancelled, abort
	default:
	}

	listTransitGatewaysOptions = tgClient.NewListTransitGatewaysOptions()
	listTransitGatewaysOptions.Limit = &perPage

	for moreData {
		select {
		case <-ctx.Done():
			log.Debugf("listTransitGatewaysByName: case <-ctx.Done()")
			return nil, ctx.Err() // we're cancelled, abort
		default:
		}

		// https://github.com/IBM/networking-go-sdk/blob/master/transitgatewayapisv1/transit_gateway_apis_v1.go#L184
		gatewayCollection, response, err = tgClient.ListTransitGatewaysWithContext(ctx, listTransitGatewaysOptions)
		if err != nil {
			return nil, fmt.Errorf("failed to list transit gateways: %w and the respose is: %s", err, response)
		}

		for _, gateway = range gatewayCollection.TransitGateways {
			select {
			case <-ctx.Done():
				log.Debugf("listTransitGatewaysByName: case <-ctx.Done()")
				return nil, ctx.Err() // we're cancelled, abort
			default:
			}

			if !matchFunc(gateway, name) {
				foundOne = true
				log.Debugf("listTransitGatewaysByName: SKIP  %s, %s", *gateway.ID, *gateway.Name)
				continue
			}

			log.Debugf("listTransitGatewaysByName: FOUND %s, %s", *gateway.ID, *gateway.Name)

			return []string{*gateway.ID}, nil
		}

		if gatewayCollection.First != nil {
			log.Debugf("listTransitGatewaysByName: First = %+v", *gatewayCollection.First.Href)
		} else {
			log.Debugf("listTransitGatewaysByName: First = nil")
		}
		if gatewayCollection.Limit != nil {
			log.Debugf("listTransitGatewaysByName: Limit = %v", *gatewayCollection.Limit)
		}
		if gatewayCollection.Next != nil {
			start, err := gatewayCollection.GetNextStart()
			if err != nil {
				log.Debugf("listTransitGatewaysByName: err = %v", err)
				return nil, fmt.Errorf("listTransitGatewaysByName: failed to GetNextStart: %w", err)
			}
			if start != nil {
				log.Debugf("listTransitGatewaysByName: start = %v", *start)
				listTransitGatewaysOptions.SetStart(*start)
			}
		} else {
			log.Debugf("listTransitGatewaysByName: Next = nil")
			moreData = false
		}
	}
	if !foundOne {
		log.Debugf("listTransitGatewaysByName: NO matching transit gateway against: %s", name)

		listTransitGatewaysOptions = tgClient.NewListTransitGatewaysOptions()
		listTransitGatewaysOptions.Limit = &perPage
		moreData = true

		for moreData {
			select {
			case <-ctx.Done():
				log.Debugf("listTransitGatewaysByName: case <-ctx.Done()")
				return nil, ctx.Err() // we're cancelled, abort
			default:
			}

			gatewayCollection, response, err = tgClient.ListTransitGatewaysWithContext(ctx, listTransitGatewaysOptions)
			if err != nil {
				return nil, fmt.Errorf("failed to list transit gateways: %w and the respose is: %s", err, response)
			}
			for _, gateway = range gatewayCollection.TransitGateways {
				select {
				case <-ctx.Done():
					log.Debugf("listTransitGatewaysByName: case <-ctx.Done()")
					return nil, ctx.Err() // we're cancelled, abort
				default:
				}

				log.Debugf("listTransitGatewaysByName: FOUND: %s, %s", *gateway.ID, *gateway.Name)
			}
			if gatewayCollection.First != nil {
				log.Debugf("listTransitGatewaysByName: First = %+v", *gatewayCollection.First.Href)
			} else {
				log.Debugf("listTransitGatewaysByName: First = nil")
			}
			if gatewayCollection.Limit != nil {
				log.Debugf("listTransitGatewaysByName: Limit = %v", *gatewayCollection.Limit)
			}
			if gatewayCollection.Next != nil {
				start, err := gatewayCollection.GetNextStart()
				if err != nil {
					log.Debugf("listTransitGatewaysByName: err = %v", err)
					return nil, fmt.Errorf("listTransitGatewaysByName: failed to GetNextStart: %w", err)
				}
				if start != nil {
					log.Debugf("listTransitGatewaysByName: start = %v", *start)
					listTransitGatewaysOptions.SetStart(*start)
				}
			} else {
				log.Debugf("listTransitGatewaysByName: Next = nil")
				moreData = false
			}
		}
	}

	return nil, nil
}

func (tg *TransitGateway) CheckConnections() (int, int, error) {
	var (
		tgClient                     *transitgatewayapisv1.TransitGatewayApisV1
		ctx                          context.Context
		cancel                       context.CancelFunc
		listConnectionsOptions       *transitgatewayapisv1.ListConnectionsOptions
		transitConnectionCollections *transitgatewayapisv1.TransitConnectionCollection
		transitConnection            transitgatewayapisv1.TransitConnection
		response                     *core.DetailedResponse
		err                          error
		perPage                      int64 = 32
		moreData                           = true
		pvsCount                           = 0
		vpcCount                           = 0
	)

	if tg.innerTg == nil {
		return 0, 0, fmt.Errorf("checkConnections innerTg is nil")
	}

	tgClient = tg.services.GetTgClient()

	ctx, cancel = tg.services.GetContextWithTimeout()
	defer cancel()

	listConnectionsOptions = tg.tgClient.NewListConnectionsOptions()
	listConnectionsOptions.SetLimit(perPage)

	for moreData {
		transitConnectionCollections, response, err = tgClient.ListConnectionsWithContext(ctx, listConnectionsOptions)
		if err != nil {
			log.Debugf("checkConnections: ListTransitGatewayConnectionsWithContext returns %v and the response is: %s", err, response)
			return 0, 0, err
		}
		for _, transitConnection = range transitConnectionCollections.Connections {
			if *tg.innerTg.ID != *transitConnection.TransitGateway.ID {
				log.Debugf("checkConnections: SKIP  %s %s %s", *transitConnection.ID, *transitConnection.Name, *transitConnection.TransitGateway.ID)
				continue
			}

			log.Debugf("checkConnections: FOUND %s, %s, %s, %s", *transitConnection.ID, *transitConnection.Name, *transitConnection.TransitGateway.ID, *transitConnection.NetworkID)

			switch *transitConnection.NetworkType {
			case transitgatewayapisv1.CreateTransitGatewayConnectionOptions_NetworkType_PowerVirtualServer:
				pvsCount++
			case transitgatewayapisv1.CreateTransitGatewayConnectionOptions_NetworkType_Vpc:
				vpcCount++
			}
			log.Debugf("checkConnections: pvsCount = %d, vpcCount = %d", pvsCount, vpcCount)
		}

		if transitConnectionCollections.First != nil {
			log.Debugf("checkConnections: First = %+v", *transitConnectionCollections.First)
		} else {
			log.Debugf("checkConnections: First = nil")
		}
		if transitConnectionCollections.Limit != nil {
			log.Debugf("checkConnections: Limit = %v", *transitConnectionCollections.Limit)
		}
		if transitConnectionCollections.Next != nil {
			start, err := transitConnectionCollections.GetNextStart()
			if err != nil {
				log.Debugf("checkConnections: err = %v", err)
				return 0, 0, fmt.Errorf("checkConnections: failed to GetNextStart: %w", err)
			}
			if start != nil {
				log.Debugf("checkConnections: start = %v", *start)
				listConnectionsOptions.SetStart(*start)
			}
		} else {
			log.Debugf("checkConnections: Next = nil")
			moreData = false
		}
	}

	return pvsCount, vpcCount, nil
}

func (tg *TransitGateway) CRN() (string, error) {
	if tg.innerTg == nil || tg.innerTg.Crn == nil {
		return "(error)", nil
	}

	return *tg.innerTg.Crn, nil
}

func (tg *TransitGateway) Name() (string, error) {
	if tg.innerTg == nil || tg.innerTg.Name == nil {
		return "(error)", nil
	}

	return *tg.innerTg.Name, nil
}

func (tg *TransitGateway) ObjectName() (string, error) {
	return tgObjectName, nil
}

func (tg *TransitGateway) Run() error {
	// Nothing to do here!
	return nil
}

func (tg *TransitGateway) CiStatus(shouldClean bool) {
}

func (tg *TransitGateway) ClusterStatus() {
	var (
		pvsCount int
		vpcCount int
		isOk     bool
		err      error
	)

	if tg.innerTg == nil {
		fmt.Printf("%s is NOTOK. Could not find a TG named %s\n", tgObjectName, tg.name)
		return
	}

	if *tg.innerTg.Status != "available" {
		fmt.Printf("%s %s is NOTOK.  The status is %s\n", tgObjectName, tg.name, *tg.innerTg.Status)
		return
	}

	isOk = true

	pvsCount, vpcCount, err = tg.CheckConnections()
	if err != nil {
		fmt.Printf("%s %s is NOTOK. Received %v checking the connections\n", tgObjectName, tg.name, err)
		isOk = false
	}

	if pvsCount == 1 {
		fmt.Printf("%s %s has a connection to a %s\n", tgObjectName, tg.name, siObjectName)
	} else {
		isOk = false
	}

	if vpcCount == 1 {
		fmt.Printf("%s %s has a connection to a %s\n", tgObjectName, tg.name, vpcObjectName)
	} else {
		isOk = false
	}

	if isOk {
		fmt.Printf("%s %s is OK.\n", tgObjectName, tg.name)
	}
}

func (tg *TransitGateway) Priority() (int, error) {
	return 90, nil
}
