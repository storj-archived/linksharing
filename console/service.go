// Copyright (C) 2020 Storj Labs, Inc.
// See LICENSE for copying information.

package console

import (
	"context"

	"github.com/zeebo/errs"
	"go.uber.org/zap"

	"storj.io/linksharing/objectmap"
	"storj.io/uplink"
	"storj.io/uplink/private/object"
)

var (
	// ErrUplink - uplink error type.
	ErrUplink = errs.Class("uplink error")
	// ErrValidate indicates that some entity validation is failed.
	ErrValidate = errs.Class("console validation error")
)

// Location represents geographical points
// in the globe.
type Location struct {
	Latitude  float64
	Longitude float64
}

// Handler implements the link sharing HTTP console.
//
// architecture: Service
type Service struct {
	log    *zap.Logger
	mapper *objectmap.IPDB
}

// NewService creates a new link sharing HTTP console.
func NewService(log *zap.Logger, mapper *objectmap.IPDB) (*Service, error) {
	return &Service{
		log:    log,
		mapper: mapper,
	}, nil
}

// GetBucketObjects returns project and a list of objects stored in a bucket.
func (service *Service) GetBucketObjects(ctx context.Context, serializedAccess, bucketName string) (project *uplink.Project, objects []*uplink.Object, err error) {
	access, err := uplink.ParseAccess(serializedAccess)
	if err != nil {
		return nil, nil, ErrValidate.Wrap(err)
	}

	project, err = uplink.OpenProject(ctx, access)
	if err != nil {
		return nil, nil, ErrUplink.Wrap(err)
	}

	// TODO: update in future
	key := ""

	objectIterator := project.ListObjects(ctx, bucketName, &uplink.ListObjectsOptions{
		Prefix: key,
		System: true,
	})
	for objectIterator.Next() {
		objects = append(objects, objectIterator.Item())
	}
	if objectIterator.Err() != nil {
		return nil, nil, ErrUplink.Wrap(objectIterator.Err())
	}

	return project, objects, nil
}

// GetSingleObjectLocations returns project, object and a list of locations where pieces of object are located.
func (service *Service) GetSingleObjectLocations(ctx context.Context, serializedAccess, bucketName, key string) (project *uplink.Project, uplinkObject *uplink.Object, locations []Location, err error) {
	access, project, uplinkObject, err := service.GetSingleObject(ctx, serializedAccess, bucketName, key)
	if err != nil {
		return nil, nil, nil, ErrValidate.Wrap(err)
	}

	ipBytes, err := object.GetObjectIPs(ctx, uplink.Config{}, access, bucketName, key)
	if err != nil {
		return nil, nil, nil, ErrUplink.Wrap(err)
	}

	for _, ip := range ipBytes {
		info, err := service.mapper.GetIPInfos(string(ip))
		if err != nil {
			service.log.Error("failed to get IP info", zap.Error(err))
			continue
		}

		location := Location{
			Latitude:  info.Location.Latitude,
			Longitude: info.Location.Longitude,
		}

		locations = append(locations, location)
	}

	return project, uplinkObject, locations, nil
}

// GetSingleObjectLocations returns uplink access string, project and specific object stored in bucket.
func (service *Service) GetSingleObject(ctx context.Context, serializedAccess, bucketName, key string) (access *uplink.Access, project *uplink.Project, uplinkObject *uplink.Object, err error) {
	access, err = uplink.ParseAccess(serializedAccess)
	if err != nil {
		return nil, nil, nil, ErrValidate.Wrap(err)
	}

	project, err = uplink.OpenProject(ctx, access)
	if err != nil {
		return nil, nil, nil, ErrUplink.Wrap(err)
	}

	uplinkObject, err = project.StatObject(ctx, bucketName, key)
	if err != nil {
		return nil, nil, nil, ErrUplink.Wrap(err)
	}

	return access, project, uplinkObject, nil
}
