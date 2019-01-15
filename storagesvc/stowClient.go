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
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fission/fission/storagesvc/progress"
	"github.com/graymeta/stow"
	_ "github.com/graymeta/stow/local"
	uuid "github.com/satori/go.uuid"
	log "github.com/sirupsen/logrus"
)

type (
	StorageType string

	storageConfig struct {
		storageType   StorageType
		localPath     string
		containerName string
		// other stuff, such as google or s3 credentials, bucket names etc
	}

	StowClient struct {
		config    *storageConfig
		location  stow.Location
		container stow.Container
		uploads   *UploadRegistry
	}
)

const (
	StorageTypeLocal StorageType = "local"
	PaginationSize   int         = 10
)

var (
	ErrNotFound                = errors.New("not found")
	ErrRetrievingItem          = errors.New("unable to retrieve item")
	ErrOpeningItem             = errors.New("unable to open item")
	ErrWritingFile             = errors.New("unable to write file")
	ErrWritingFileIntoResponse = errors.New("unable to copy item into http response")
)

func MakeStowClient(storageType StorageType, storagePath string, containerName string) (*StowClient, error) {
	if storageType != StorageTypeLocal {
		return nil, errors.New("Storage types other than 'local' are not implemented")
	}

	config := &storageConfig{
		storageType:   storageType,
		localPath:     storagePath,
		containerName: containerName,
	}

	stowClient := &StowClient{
		config:  config,
		uploads: NewUploadRegistry(),
	}

	cfg := stow.ConfigMap{"path": config.localPath}
	loc, err := stow.Dial("local", cfg)
	if err != nil {
		log.WithError(err).Error("Error initializing storage")
		return nil, err
	}
	stowClient.location = loc

	con, err := loc.CreateContainer(config.containerName)
	if os.IsExist(err) {
		var cons []stow.Container
		var cursor string

		// use location.Containers to find containers that match the prefix (container name)
		cons, cursor, err = loc.Containers(config.containerName, stow.CursorStart, 1)
		if err == nil {
			if !stow.IsCursorEnd(cursor) {
				// Should only have one storage container
				err = errors.New("Found more than one matched storage containers")
			} else {
				con = cons[0]
			}
		}
	}
	if err != nil {
		log.WithError(err).Error("Error initializing storage")
		return nil, err
	}
	stowClient.container = con

	return stowClient, nil
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

	item, err := client.container.Put(uploadName, r, int64(fileSize), nil)
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

func (client *StowClient) status(uploadName string) (progress.Counter, int64, error) {
	counter, size := client.uploads.get(uploadName)

	if counter != nil {
		return counter, size, nil
	}

	// HACK(adamb) Delete this uploadName -> itemID logic once any of:
	//     1) we stop using stow's local backend
	//     2) https://github.com/graymeta/stow/issues/170 is closed
	//     3) https://github.com/graymeta/stow/pull/175 is merged
	var itemID string
	if strings.HasPrefix(uploadName, "/") {
		itemID = uploadName
	} else {
		itemID = filepath.Join(client.container.ID(), uploadName)
	}

	item, err := client.container.Item(itemID)
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
func (client *StowClient) copyFileToStream(fileId string, w io.Writer) error {
	item, err := client.container.Item(fileId)
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
		log.WithError(err).Printf("Error copying file: %s into httpresponse", fileId)
		return ErrWritingFileIntoResponse
	}

	log.Debugf("successfully wrote file: %s into httpresponse", fileId)
	return nil
}

// removeFileByID deletes the file from storage
func (client *StowClient) removeFileByID(itemID string) error {
	return client.container.RemoveItem(itemID)
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
		items, cursor, err = client.container.Items(stow.NoPrefix, cursor, PaginationSize)
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
