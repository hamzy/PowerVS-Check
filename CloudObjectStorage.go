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

	"github.com/IBM/ibm-cos-sdk-go/aws"
	"github.com/IBM/ibm-cos-sdk-go/aws/awserr"
	"github.com/IBM/ibm-cos-sdk-go/aws/credentials/ibmiam"
	"github.com/IBM/ibm-cos-sdk-go/aws/session"
	"github.com/IBM/ibm-cos-sdk-go/service/s3"
	"github.com/IBM/ibm-cos-sdk-go/service/s3/s3manager"

	// https://raw.githubusercontent.com/IBM/platform-services-go-sdk/refs/heads/main/resourcecontrollerv2/resource_controller_v2.go
	"github.com/IBM/platform-services-go-sdk/resourcecontrollerv2"
)

type CloudObjectStorage struct {
	name string

	region string

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

	cos.region, err = services.GetMetadata().GetVPCRegion()
	if err != nil {
		return []RunnableObject{cos}, []error{err}
	}
	log.Debugf("NewCloudObjectStorage: region = %s", cos.region)

	cos.serviceEndpoint = fmt.Sprintf("s3.%s.cloud-object-storage.appdomain.cloud", cos.region)

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

		err = cos.createClients()
		if err != nil {
			return nil, []error{fmt.Errorf("Error: NewCloudObjectStorage createClients returns %v", err)}
		}

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

func (cos *CloudObjectStorage) createClients() error {
	var (
		options session.Options
		err     error
	)

	if cos.innerCos == nil {
		return fmt.Errorf("Error: createClients called on nil CloudObjectStorage")
	}

	options.Config = *aws.NewConfig().
		WithRegion(cos.region).
		WithEndpoint(cos.serviceEndpoint).
		WithCredentials(ibmiam.NewStaticCredentials(
			aws.NewConfig(),
			"https://iam.cloud.ibm.com/identity/token",
			cos.services.GetApiKey(),
			*cos.innerCos.GUID,
		)).
		WithS3ForcePathStyle(true)

	// https://github.com/IBM/ibm-cos-sdk-go/blob/master/aws/session/session.go#L268
	cos.awsSession, err = session.NewSessionWithOptions(options)
	if err != nil {
		log.Fatalf("Error: NewSessionWithOptions returns %v", err)
		return err
	}
	log.Debugf("createClients: cos.awsSession = %+v", cos.awsSession)
	if cos.awsSession == nil {
		log.Fatalf("Error: cos.awsSession is nil")
		return fmt.Errorf("Error: cos.awsSession is nil")
	}

	cos.s3Client = s3.New(cos.awsSession)
	log.Debugf("createClients: cos.s3Client = %+v", cos.s3Client)
	if cos.s3Client == nil {
		log.Fatalf("Error: cos.s3Client is nil")
		return fmt.Errorf("Error: cos.s3Client is nil")
	}

	return err
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

func isBucketNotFound(err interface{}) bool {
	log.Debugf("isBucketNotFound: err = %v", err)
	log.Debugf("isBucketNotFound: err.(type) = %T", err)

	if err == nil {
		return false
	}

	// vet: ./CloudObjectStorage.go:443:14: use of .(type) outside type switch
	// if _, ok := err.(type); !ok {

	switch err.(type) {
	case s3.RequestFailure:
		log.Debugf("isBucketNotFound: err.(type) s3.RequestFailure")
		if reqerr, ok := err.(s3.RequestFailure); ok {
			log.Debugf("isBucketNotFound: reqerr.Code() = %v", reqerr.Code())
			switch reqerr.Code() {
			case s3.ErrCodeNoSuchBucket:
				return true
			case "NotFound":
				return true
			case "Forbidden":
				return true
			}
			log.Debugf("isBucketNotFound: continuing")
		} else {
			log.Debugf("isBucketNotFound: s3.RequestFailure !ok")
		}
	case awserr.Error:
		log.Debugf("isBucketNotFound: err.(type) awserr.Error")
		if reqerr, ok := err.(awserr.Error); ok {
			log.Debugf("isBucketNotFound: reqerr.Code() = %v", reqerr.Code())
			switch reqerr.Code() {
			case s3.ErrCodeNoSuchBucket:
				return true
			case "NotFound":
				return true
			case "Forbidden":
				return true
			}
			log.Debugf("isBucketNotFound: continuing")
		} else {
			log.Debugf("isBucketNotFound: s3.RequestFailure !ok")
		}
	}

	// @TODO investigate
	switch s3Err := err.(type) {
	case awserr.Error:
		if s3Err.Code() == "NoSuchBucket" {
			return true
		}
		origErr := s3Err.OrigErr()
		if origErr != nil {
			return isBucketNotFound(origErr)
		}
	case s3manager.Error:
		if s3Err.OrigErr != nil {
			return isBucketNotFound(s3Err.OrigErr)
		}
	case s3manager.Errors:
		if len(s3Err) == 1 {
			return isBucketNotFound(s3Err[0])
		}
		// Weird: This does not match?!
		// case s3.RequestFailure:
	}

	return false
}

func (cos *CloudObjectStorage) examineCOS() error {
	var (
		ctx              context.Context
		cancel           context.CancelFunc
		bucket           string
		headBucketInput  *s3.HeadBucketInput
		headBucketOutput *s3.HeadBucketOutput
		//		listBucketOutput  *s3.ListBucketsOutput
		listObjectsInput  *s3.ListObjectsInput
		listObjectsOutput *s3.ListObjectsOutput
		s3Object          *s3.Object
		expectedObjects   = map[string]bool{
			"master-0": false,
			"master-1": false,
			"master-2": false,
		}
		allFound bool
		err      error
	)

	ctx, cancel = cos.services.GetContextWithTimeout()
	defer cancel()

	bucket = fmt.Sprintf("%s-bootstrap-ign", cos.services.GetMetadata().GetInfraID())
	log.Debugf("examineCOS: bucket = %s", bucket)

	headBucketInput = &s3.HeadBucketInput{
		Bucket: aws.String(bucket),
	}
	headBucketOutput, err = cos.s3Client.HeadBucketWithContext(ctx, headBucketInput)
	if isBucketNotFound(err) {
		return fmt.Errorf("bucket %s not found!", bucket)
	}
	log.Debugf("examineCOS: headBucketOutput = %+v", *headBucketOutput)

	//	listBucketOutput, err = cos.s3Client.ListBucketsWithContext(ctx, &s3.ListBucketsInput{})
	//	if err != nil {
	//		return err
	//	}
	//	log.Debugf("examineCOS: listBucketOutput = %+v", *listBucketOutput)

	listObjectsInput = &s3.ListObjectsInput{
		Bucket: aws.String(bucket),
	}
	listObjectsOutput, err = cos.s3Client.ListObjectsWithContext(ctx, listObjectsInput)
	if err != nil {
		return err
	}
	log.Debugf("examineCOS: listObjectsOutput = %+v", *listObjectsOutput)

	for _, s3Object = range listObjectsOutput.Contents {
		fmt.Printf("Found %s (size %d) in %s\n", *s3Object.Key, int64(*s3Object.Size), bucket)

		for expectedObjectKey := range expectedObjects {
			if strings.Contains(*s3Object.Key, expectedObjectKey) {
				expectedObjects[expectedObjectKey] = true
			}
		}
	}
	log.Debugf("examineCOS: expectedObjects = %+v\n", expectedObjects)

	allFound = true
	for expectedObjectKey := range expectedObjects {
		if !expectedObjects[expectedObjectKey] {
			allFound = false
		}
	}
	if !allFound {
		return fmt.Errorf("Did not find all master ignition files")
	}

	return nil
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
	var (
		isOk bool
		err  error
	)

	if cos.innerCos == nil {
		fmt.Printf("%s: Could not find a COS named %s\n", cosObjectName, cos.name)
		return
	}

	isOk = true

	if *cos.innerCos.State != "active" {
		isOk = false
		fmt.Printf("%s %s is NOTOK state is not active (%s)\n", cosObjectName, cos.name, *cos.innerCos.State)
	}

	err = cos.examineCOS()
	if err != nil {
		isOk = false
		fmt.Printf("%s %s is NOTOK (%v)\n", cosObjectName, cos.name, err)
	}

	if isOk {
		fmt.Printf("%s %s is OK.\n", cosObjectName, cos.name)
	}
}

func (cos *CloudObjectStorage) Priority() (int, error) {
	return 60, nil
}
