/*
Copyright 2017 The Fission Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package storagesvc

import (
	"errors"
	"io"
	"path/filepath"
	"time"

	multierror "github.com/hashicorp/go-multierror"

	"github.com/fission/fission/storagesvc/progress"
	"github.com/graymeta/stow"
	_ "github.com/graymeta/stow/local"
	uuid "github.com/satori/go.uuid"
	log "github.com/sirupsen/logrus"
)

type (
	StowClient struct {
		writeContainer stow.Container

		readContainers []stow.Container
		uploads        *UploadRegistry
	}
)

const (
	PaginationSize int = 10
)

var (
	ErrNotFound                = errors.New("not found")
	ErrRetrievingItem          = errors.New("unable to retrieve item")
	ErrOpeningItem             = errors.New("unable to open item")
	ErrWritingFile             = errors.New("unable to write file")
	ErrWritingFileIntoResponse = errors.New("unable to copy item into http response")
)

func ResolveContainer(kind, containerName string, cfg stow.ConfigMap) (stow.Container, error) {
	loc, err := stow.Dial(kind, cfg)
	if err != nil {
		log.WithError(err).Error("Error initializing storage kind: " + kind)
		return nil, err
	}

	// use location.Containers to find containers that match the prefix (container name)
	con, err := loc.Container(containerName)
	if err == nil {
		return con, nil
	}

	con, err = loc.CreateContainer(containerName)
	if err != nil {
		log.WithError(err).Error("Error creating: " + containerName)
		return nil, err
	}

	return con, nil
}

func MakeStowClient(readWriteContainer stow.Container, readOnlyContainers ...stow.Container) *StowClient {
	return &StowClient{
		writeContainer: readWriteContainer,
		readContainers: append([]stow.Container{readWriteContainer}, readOnlyContainers...),

		uploads: NewUploadRegistry(),
	}
}

// putFile writes the file on the storage
func (client *StowClient) putFile(reader io.Reader, fileSize int64, uploadName string) (string, error) {
	if uploadName == "" {
		// This is not the item ID (that's returned by Put)
		// should we just use handler.Filename? what are the constraints here?
		uploadName = uuid.NewV4().String()
	}

	r := client.uploads.declare(uploadName, fileSize, reader)
	defer client.uploads.remove(uploadName, r)

	item, err := client.writeContainer.Put(uploadName, r, int64(fileSize), nil)
	if err != nil {
		log.WithError(err).Errorf("Error writing file: %s on storage, size %d", uploadName, fileSize)
		return "", ErrWritingFile
	}

	log.Debugf("Successfully wrote file:%s on storage", uploadName)
	return item.ID(), nil
}

type completedUpload int64

func (cu completedUpload) N() int64 {
	return int64(cu)
}

func (cu completedUpload) Extra() interface{} {
	return nil
}

func (cu completedUpload) Err() error {
	return io.EOF
}

func (client *StowClient) setStatusExtra(uploadName string, extra interface{}) error {
	return client.uploads.setExtra(uploadName, extra)
}

func (client *StowClient) findItemForUploadName(uploadName string) (stow.Container, stow.Item, error) {
	merr := &multierror.Error{}
	for _, container := range client.readContainers {
		itemID := filepath.Join(container.ID(), uploadName)

		item, err := container.Item(itemID)
		if err == nil {
			return container, item, nil
		}
		if err != stow.ErrNotFound {
			merr.Errors = append(merr.Errors, err)
		}
	}

	merr.Errors = append(merr.Errors, stow.ErrNotFound)

	return nil, nil, merr.ErrorOrNil()
}

func (client *StowClient) status(uploadName string) (progress.Counter, int64, error) {
	counter, size := client.uploads.get(uploadName)

	if counter != nil {
		return counter, size, nil
	}

	_, item, err := client.findItemForUploadName(uploadName)
	if err != nil {
		if err == stow.ErrNotFound {
			return nil, -1, ErrNotFound
		} else {
			return nil, -1, err
		}
	}

	size, err = item.Size()
	if err != nil {
		return nil, -1, err
	}

	return completedUpload(size), size, nil
}

// copyFileToStream gets the file contents into a stream
func (client *StowClient) copyFileToStream(uploadName string, w io.Writer) error {
	_, item, err := client.findItemForUploadName(uploadName)
	if err != nil {
		if err == stow.ErrNotFound {
			return ErrNotFound
		} else {
			return ErrRetrievingItem
		}
	}

	f, err := item.Open()
	if err != nil {
		return ErrOpeningItem
	}
	defer f.Close()

	_, err = io.Copy(w, f)
	if err != nil {
		log.WithError(err).Printf("Error copying file: %s into httpresponse", uploadName)
		return ErrWritingFileIntoResponse
	}

	log.Debugf("successfully wrote file: %s into httpresponse", uploadName)
	return nil
}

// removeFileByID deletes the file from storage
func (client *StowClient) removeFileByID(uploadName string) error {
	container, _, err := client.findItemForUploadName(uploadName)
	if err != nil {
		return err
	}
	return container.RemoveItem(uploadName)
}

// filter defines an interface to filter out items from a set of items
type filter func(stow.Item, interface{}) bool

// This method returns all items in a container, filtering out items based on the filter function passed to it
func (client *StowClient) getItemIDsWithFilter(filterFunc filter, filterFuncParam interface{}) ([]string, error) {
	cursor := stow.CursorStart
	var items []stow.Item
	var err error

	archiveIDList := make([]string, 0)

	for {
		items, cursor, err = client.writeContainer.Items(stow.NoPrefix, cursor, PaginationSize)
		if err != nil {
			log.WithError(err).Error("Error getting items from container")
			return nil, err
		}

		for _, item := range items {
			isItemFilterable := filterFunc(item, filterFuncParam)
			if isItemFilterable {
				continue
			}
			archiveIDList = append(archiveIDList, item.ID())
		}

		if stow.IsCursorEnd(cursor) {
			break
		}
	}

	return archiveIDList, nil
}

// filterItemCreatedAMinuteAgo is one type of filter function that filters out items created less than a minute ago.
// More filter functions can be written if needed, as long as they are of type filter
func filterItemCreatedAMinuteAgo(item stow.Item, currentTime interface{}) bool {
	itemLastModTime, _ := item.LastMod()
	if currentTime.(time.Time).Sub(itemLastModTime) < 1*time.Minute {
		log.Debugf("item: %s created less than a minute ago: %v", item.ID(), itemLastModTime)
		return true
	}
	return false
}
