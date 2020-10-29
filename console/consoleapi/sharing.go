// Copyright (C) 2020 Storj Labs, Inc.
// See LICENSE for copying information.

package consoleapi

import (
	"html/template"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/spacemonkeygo/monkit/v3"
	"github.com/zeebo/errs"
	"go.uber.org/zap"

	"storj.io/common/memory"
	"storj.io/common/ranger/httpranger"
	"storj.io/linksharing/console"
	"storj.io/linksharing/objectranger"
)

const (
	contentType = "Content-Type"

	plain = "text/plain"
	html  = "text/html"
)

var mon = monkit.Package()

// ErrSharingAPI - console link sharing api error type.
var ErrSharingAPI = errs.Class("linksharing console web error")

// Sharing is an api controller that exposes all link sharing related api.
type Sharing struct {
	log *zap.Logger

	service   *console.Service
	templates SharingTemplates
}

// SharingTemplates holds all templates needed to be rendered.
type SharingTemplates struct {
	List         *template.Template
	SingleObject *template.Template
	NotFound     *template.Template
}

// BucketFiles holds all data needed to render prefix-list template.
type BucketFiles struct {
	BucketName string
	Files      []File
}

// File holds all data needed to render file row.
type File struct {
	Name   string
	URL    string
	Size   string
	Prefix bool
}

// SingleFile holds all data needed to render object map of a file.
type SingleFile struct {
	Name      string
	Size      string
	Locations []console.Location
	Pieces    int64
}

// NewSharing is a constructor for a link sharing controller.
func NewSharing(log *zap.Logger, service *console.Service, templates SharingTemplates) *Sharing {
	return &Sharing{
		log:       log,
		templates: templates,
		service:   service,
	}
}

// BucketFiles handles link sharing bucket files list API requests.
func (sharing *Sharing) BucketFiles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var err error
	defer mon.Task()(&ctx)(&err)

	w.Header().Set(contentType, html)

	serializedAccess := getSerializedAccess(w, r)
	bucketName := getBucketName(w, r)

	project, objects, err := sharing.service.GetBucketObjects(ctx, serializedAccess, bucketName)
	if err != nil {
		sharing.log.Error("could not get project files", zap.Error(err))
		if console.ErrValidate.Has(err) {
			http.Error(w, http.StatusText(http.StatusBadRequest)+": serialized access parameter is invalid", http.StatusBadRequest)
			return
		}
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	defer func() {
		if err := project.Close(); err != nil {
			sharing.log.With(zap.Error(err)).Warn("unable to close project")
		}
	}()

	var bucketFiles BucketFiles
	for _, object := range objects {
		name := object.Key[len(""):]
		bucketFiles.Files = append(bucketFiles.Files, File{
			Name:   name,
			URL:    bucketName + "/" + name,
			Size:   memory.Size(object.System.ContentLength).Base10String(),
			Prefix: object.IsPrefix,
		})
	}

	bucketFiles.BucketName = bucketName

	err = sharing.templates.List.Execute(w, bucketFiles)
	if err != nil {
		sharing.log.With(zap.Error(err)).Warn("failed to execute template")
	}
}

// File handles link sharing single file API requests.
func (sharing *Sharing) File(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var err error
	defer mon.Task()(&ctx)(&err)

	w.Header().Set(contentType, html)

	serializedAccess := getSerializedAccess(w, r)
	bucketName := getBucketName(w, r)
	fileName := getFileName(w, r)

	project, object, locations, err := sharing.service.GetSingleObjectLocations(ctx, serializedAccess, bucketName, fileName)
	if err != nil {
		sharing.log.Error("could not get project files", zap.Error(err))
		if console.ErrValidate.Has(err) {
			http.Error(w, http.StatusText(http.StatusBadRequest)+": serialized access parameter is invalid", http.StatusBadRequest)
			return
		}
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	defer func() {
		if err := project.Close(); err != nil {
			sharing.log.With(zap.Error(err)).Warn("unable to close project")
		}
	}()

	_, download := r.URL.Query()["download"]
	_, view := r.URL.Query()["view"]
	if !download && !view {
		singleFile := SingleFile{
			Name:      object.Key,
			Size:      memory.Size(object.System.ContentLength).Base10String(),
			Locations: locations,
			Pieces:    int64(len(locations)),
		}

		err = sharing.templates.SingleObject.Execute(w, singleFile)
		if err != nil {
			sharing.log.With(zap.Error(err)).Warn("failed to execute template")
		}

		return
	}

	if download {
		w.Header().Set("Content-Disposition", "attachment; filename=\""+fileName+"\"")
	}

	w.Header().Set(contentType, plain)

	httpranger.ServeContent(ctx, w, r, fileName, object.System.Created, objectranger.NewObjectRanger(project, object, bucketName))
}

// OpenFile handles link sharing open file API requests.
func (sharing *Sharing) OpenFile(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var err error
	defer mon.Task()(&ctx)(&err)

	w.Header().Set(contentType, plain)

	serializedAccess := getSerializedAccess(w, r)
	bucketName := getBucketName(w, r)
	fileName := getFileName(w, r)

	_, project, object, err := sharing.service.GetSingleObject(ctx, serializedAccess, bucketName, fileName)
	if err != nil {
		sharing.log.Error("could not get project files", zap.Error(err))
		if console.ErrValidate.Has(err) {
			http.Error(w, http.StatusText(http.StatusBadRequest)+": serialized access parameter is invalid", http.StatusBadRequest)
			return
		}
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	defer func() {
		if err := project.Close(); err != nil {
			sharing.log.With(zap.Error(err)).Warn("unable to close project")
		}
	}()

	httpranger.ServeContent(ctx, w, r, fileName, object.System.Created, objectranger.NewObjectRanger(project, object, bucketName))
}

func getSerializedAccess(w http.ResponseWriter, r *http.Request) string {
	params := mux.Vars(r)

	serializedAccess, ok := params["serialized-access"]
	if !ok {
		http.Error(w, http.StatusText(http.StatusBadRequest)+": serialized access parameter is missing", http.StatusBadRequest)
		return ""
	}

	return serializedAccess
}

func getBucketName(w http.ResponseWriter, r *http.Request) string {
	params := mux.Vars(r)

	bucketName, ok := params["bucket-name"]
	if !ok {
		http.Error(w, http.StatusText(http.StatusBadRequest)+": bucket name parameter is missing", http.StatusBadRequest)
		return ""
	}

	return bucketName
}

func getFileName(w http.ResponseWriter, r *http.Request) string {
	params := mux.Vars(r)

	fileName, ok := params["file-name"]
	if !ok {
		http.Error(w, http.StatusText(http.StatusBadRequest)+": file name parameter is missing", http.StatusBadRequest)
		return ""
	}

	return fileName
}
