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

	"github.com/IBM/ibm-cos-sdk-go/aws/session"
	"github.com/IBM/ibm-cos-sdk-go/service/s3"

	// https://raw.githubusercontent.com/IBM/platform-services-go-sdk/refs/heads/main/resourcecontrollerv2/resource_controller_v2.go
	"github.com/IBM/platform-services-go-sdk/resourcecontrollerv2"
)

type CloudObjectStorage struct {
	name string

	services *Services

	innerCos *resourcecontrollerv2.ResourceInstance

	awsSession *session.Session

	s3Client *s3.S3

	serviceEndpoint string
}

const (
	cosObjectName = "Cloud Object Storage"
)

func NewCloudObjectStorage(services *Services) ([]RunnableObject, []error) {
	var (
		cosName        string
		region         string
		controllerSvc  *resourcecontrollerv2.ResourceControllerV2
		ctx            context.Context
		cancel         context.CancelFunc
		foundInstances []string
		cos            *CloudObjectStorage
		coses          []RunnableObject
		errs           []error
		idxCos         int
		err            error
	)

	cos = &CloudObjectStorage{
		name:            "",
		services:        services,
		innerCos:        nil,
		serviceEndpoint: "",
	}

	cosName, err = services.GetMetadata().GetObjectName(RunnableObject(&CloudObjectStorage{}))
	if err != nil {
		return []RunnableObject{cos}, []error{err}
	}
	if cosName == "" {
		return nil, nil
	}
	cos.name = cosName

	region = services.GetMetadata().GetRegion()
	log.Debugf("NewCloudObjectStorage: region = %s", region)

	cos.serviceEndpoint = fmt.Sprintf("s3.%s.cloud-object-storage.appdomain.cloud", region)

	controllerSvc = services.GetControllerSvc()

	ctx, cancel = services.GetContextWithTimeout()
	defer cancel()

	if fUseTagSearch {
		foundInstances, err = listByTag(TagTypeCloudObjectStorage, services)
	} else {
		foundInstances, err = findCos(cosName, controllerSvc, ctx)
	}
	if err != nil {
		return []RunnableObject{cos}, []error{err}
	}
	log.Debugf("NewCloudObjectStorage: foundInstances = %+v, err = %v", foundInstances, err)

	coses = make([]RunnableObject, 1)
	errs = make([]error, 1)

	idxCos = 0
	coses[idxCos] = cos

	if len(foundInstances) == 0 {
		errs[idxCos] = fmt.Errorf("Unable to find %s named %s", cosObjectName, cosName)
	}

	for _, instanceID := range foundInstances {
		var (
			getOptions   *resourcecontrollerv2.GetResourceInstanceOptions
			innerCos     *resourcecontrollerv2.ResourceInstance
			innerCosName string
		)

		getOptions = controllerSvc.NewGetResourceInstanceOptions(instanceID)

		innerCos, _, err = controllerSvc.GetResourceInstanceWithContext(ctx, getOptions)
		if err != nil {
			// Just in case.
			innerCos = nil
		}

		if innerCos != nil {
			innerCosName = *innerCos.Name
		} else {
			innerCosName = cosName
		}
		cos.name = innerCosName
		cos.innerCos = innerCos

		if idxCos > 0 {
			log.Debugf("NewCloudObjectStorage: appending to coses")

			coses = append(coses, cos)
			errs = append(errs, err)
		} else {
			log.Debugf("NewCloudObjectStorage: altering first coses")

			coses[idxCos] = cos
			errs[idxCos] = err
		}

		idxCos++
	}

	return coses, errs
}

func findCos(name string, controllerSvc *resourcecontrollerv2.ResourceControllerV2, ctx context.Context) ([]string, error) {
	var (
		// https://github.com/IBM/platform-services-go-sdk/blob/main/resourcecontrollerv2/resource_controller_v2.go#L3086
		options *resourcecontrollerv2.ListResourceInstancesOptions
		perPage int64 = 64
		// https://github.com/IBM/platform-services-go-sdk/blob/main/resourcecontrollerv2/resource_controller_v2.go#L4525-L4534
		resources *resourcecontrollerv2.ResourceInstancesList
		err       error
		moreData  = true
	)

	log.Debugf("findCOS: name = %s", name)

	options = controllerSvc.NewListResourceInstancesOptions()
	options.Limit = &perPage
	options.SetType("service_instance")
	//	options.SetResourcePlanID(cosStandardResourceID)
	//	options.SetResourcePlanID(cosOneRateResourceID)		// @BUG does not produce a list?

	for moreData {
		select {
		case <-ctx.Done():
			log.Debugf("findCOS: case <-ctx.Done()")
			return nil, ctx.Err() // we're cancelled, abort
		default:
		}

		// https://github.com/IBM/platform-services-go-sdk/blob/main/resourcecontrollerv2/resource_controller_v2.go#L173
		resources, _, err = controllerSvc.ListResourceInstancesWithContext(ctx, options)
		if err != nil {
			return nil, fmt.Errorf("failed to list COS instances: %w", err)
		}
		log.Debugf("findCOS: RowsCount %v", *resources.RowsCount)

		for _, instance := range resources.Resources {
			select {
			case <-ctx.Done():
				log.Debugf("findCOS: case <-ctx.Done()")
				return nil, ctx.Err() // we're cancelled, abort
			default:
			}

			if *instance.Name != name {
				log.Debugf("findCOS: SKIP %s %s", *instance.Name, *instance.GUID)
				continue
			}

			log.Debugf("findCOS: FOUND %s %s", *instance.Name, *instance.GUID)
			return []string{*instance.GUID}, nil
		}

		if resources.NextURL != nil {
			start, err := resources.GetNextStart()
			if err != nil {
				log.Debugf("findCOS: err = %v", err)
				return nil, fmt.Errorf("failed to GetNextStart: %w", err)
			}
			if start != nil {
				log.Debugf("findCOS: start = %v", *start)
				options.SetStart(*start)
			}
		} else {
			log.Debugf("findCOS: NextURL = nil")
			moreData = false
		}
	}

	return []string{}, nil
}

func (cos *CloudObjectStorage) CRN() (string, error) {
	if cos.innerCos == nil || cos.innerCos.CRN == nil {
		return "(error)", nil
	}

	return *cos.innerCos.CRN, nil
}

func (cos *CloudObjectStorage) Name() (string, error) {
	if cos.innerCos == nil || cos.innerCos.Name == nil {
		return "(error)", nil
	}

	return *cos.innerCos.Name, nil
}

func (cos *CloudObjectStorage) ObjectName() (string, error) {
	return cosObjectName, nil
}

func (cos *CloudObjectStorage) Run() error {
	// Nothing to do here.
	return nil
}

func (cos *CloudObjectStorage) CiStatus(shouldClean bool) {
}

func (cos *CloudObjectStorage) ClusterStatus() {
	if cos.innerCos == nil {
		fmt.Printf("%s: Could not find a COS named %s\n", cosObjectName, cos.name)
		return
	}

	if *cos.innerCos.State != "active" {
		fmt.Printf("%s %s is NOTOK (%s)\n", cosObjectName, cos.name, *cos.innerCos.State)
		return
	}

	fmt.Printf("%s %s is OK.\n", cosObjectName, cos.name)
}

func (cos *CloudObjectStorage) Priority() (int, error) {
	return 60, nil
}
