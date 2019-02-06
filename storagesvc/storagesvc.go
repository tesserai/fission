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
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"net/http"
	"net/textproto"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/graymeta/stow"
	"github.com/graymeta/stow/google"
	"github.com/graymeta/stow/local"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"go.opencensus.io/plugin/ochttp"

	"github.com/fission/fission"
	"github.com/fission/fission/storagesvc/multipartformdata"
	"github.com/fission/fission/storagesvc/progress"
)

const (
	ConfigProvider  = "fission/storagesvc/provider"
	ConfigContainer = "fission/storagesvc/container"

	ConfigLocalKeyPath = local.ConfigKeyPath

	ConfigGCSJSON      = google.ConfigJSON
	ConfigGCSProjectId = google.ConfigProjectId
	ConfigGCSScopes    = google.ConfigScopes
)

type (
	StorageService struct {
		storageClient *StowClient
		port          int
	}

	UploadStatus struct {
		Extra  interface{} `json:"extra"`
		Status string      `json:"status"`
		N      int64       `json:"n"`
		Size   int64       `json:"size"`
		Err    error       `json:"error,omitempty"`
	}

	UploadResponse struct {
		ID string `json:"id"`
	}
)

func hexdigest(h hash.Hash) string {
	return fmt.Sprintf("%x", h.Sum(nil))
}

// Handle multipart file uploads.
func (ss *StorageService) uploadHandler(w http.ResponseWriter, r *http.Request) {
	// stow wants the file size, but that's different from the
	// content length, the content length being the size of the
	// encoded file in the HTTP request. So we require an
	// "X-File-Size" header in bytes.

	var err error
	fileSize := -1
	fileSizeS, ok := r.Header["X-File-Size"]
	if ok {
		fileSize, err = strconv.Atoi(fileSizeS[0])
		if err != nil {
			log.WithError(err).Errorf("Error parsing x-file-size: '%v'", fileSizeS)
			http.Error(w, "bad X-File-Size header", http.StatusBadRequest)
			return
		}
	}

	expectedFileSHA256s, ok := r.Header["X-File-Sha256"]
	expectedFileSHA256 := ""
	if ok {
		expectedFileSHA256 = expectedFileSHA256s[0]
	}
	uploadName, ok := mux.Vars(r)["archiveID"]

	mr, err := r.MultipartReader()
	if err != nil {
		log.WithError(err).Error("error parsing multipart form")
	}

	var digest string
	visitor := func(filename string, header textproto.MIMEHeader, reader io.Reader) (func() error, error) {
		log.Infof("Handling upload for %v", filename)

		if filename != "uploaded" {
			return nil, fmt.Errorf("Unexpected file: %s", filename)
		}

		// TODO: allow headers to add more metadata (e.g. environment
		// and function metadata)

		hasher := sha256.New()
		digestReader := io.TeeReader(reader, hasher)
		_, err = ss.storageClient.putFile(digestReader, int64(fileSize), uploadName)
		if err != nil {
			return nil, err
		}
		digest = hexdigest(hasher)

		return func() error { return ss.storageClient.removeFileByID(uploadName) }, nil
	}

	err = multipartformdata.ReadForm(mr, visitor)
	if err != nil {
		log.WithError(err).Error("error parsing multipart form")
		http.Error(w, "Error saving uploaded file", http.StatusInternalServerError)
		return
	}

	// handle upload
	if digest == "" {
		log.WithError(err).Error("missing upload file")
		http.Error(w, "missing upload file", http.StatusBadRequest)
		return
	}

	if expectedFileSHA256 != "" && digest != expectedFileSHA256 {
		log.Errorf("Did not match expected X-File-Sha256 %s, got %s", expectedFileSHA256, digest)
		http.Error(w, "Didn't match expected X-File-Sha256", http.StatusBadRequest)
		return
	}

	// respond with an ID that can be used to retrieve the file
	ur := &UploadResponse{
		ID: uploadName,
	}
	resp, err := json.Marshal(ur)
	if err != nil {
		http.Error(w, "Error marshaling response", http.StatusInternalServerError)
		return
	}
	w.Write(resp)
}

func (ss *StorageService) getIdFromRequest(r *http.Request) (string, error) {
	values := r.URL.Query()
	ids, ok := values["id"]
	if !ok || len(ids) == 0 {
		return "", errors.New("Missing `id' query param")
	}

	id := ids[0]
	return filepath.Base(id), nil
}

func (ss *StorageService) deleteHandler(w http.ResponseWriter, r *http.Request) {
	// get id from request
	fileId, err := ss.getIdFromRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	err = ss.storageClient.removeFileByID(fileId)
	if err != nil {
		msg := fmt.Sprintf("Error deleting item: %v", err)
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (ss *StorageService) downloadHandler(w http.ResponseWriter, r *http.Request) {
	// get id from request
	fileId, err := ss.getIdFromRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Get the file (called "item" in stow's jargon), open it,
	// stream it to response
	err = ss.storageClient.copyFileToStream(fileId, w)
	if err != nil {
		log.WithError(err).Errorf("Error getting item id '%v'", fileId)
		if err == ErrNotFound {
			http.Error(w, "Error retrieving item: not found", http.StatusNotFound)
		} else if err == ErrRetrievingItem {
			http.Error(w, "Error retrieving item", http.StatusBadRequest)
		} else if err == ErrOpeningItem {
			http.Error(w, "Error opening item", http.StatusBadRequest)
		} else if err == ErrWritingFileIntoResponse {
			http.Error(w, "Error writing response", http.StatusInternalServerError)
		}
		return
	}
}

func uploadStatusForCounter(counter progress.Counter, size int64) UploadStatus {
	var statusStr string
	if counter.Err() == io.EOF {
		statusStr = "done"
	} else {
		statusStr = "pending"
	}

	return UploadStatus{
		Extra:  counter.Extra(),
		Status: statusStr,
		Size:   size,
		N:      counter.N(),
		Err:    counter.Err(),
	}
}

func uploadStatusForProgress(progress progress.Progress) UploadStatus {
	var statusStr string
	if progress.Complete() {
		statusStr = "done"
	} else {
		statusStr = "pending"
	}

	return UploadStatus{
		Extra:  progress.Extra(),
		Status: statusStr,
		Size:   progress.Size(),
		N:      progress.N(),
	}
}

func (ss *StorageService) eventsHandler(w http.ResponseWriter, r *http.Request) {
	// get id from request
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, `{"error": "streaming not supported"}`, http.StatusInternalServerError)
		return
	}

	fileId, err := ss.getIdFromRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	counter, size, err := ss.storageClient.status(fileId)
	if err != nil {
		if err == ErrNotFound {
			http.Error(w, `{"error": "not found"}`, http.StatusNotFound)
		} else if err == ErrRetrievingItem {
			http.Error(w, `{"error": "bad request"}`, http.StatusBadRequest)
		} else {
			http.Error(w, fmt.Sprintf(`{"error": %#v}`, err.Error()), http.StatusInternalServerError)
		}
		return
	}

	jsonEncoder := json.NewEncoder(w)
	header := w.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	progressChan := progress.NewTicker(ctx, counter, size, 1*time.Second)
	for p := range progressChan {
		_, err = io.WriteString(w, "event: status\ndata: ")
		if err != nil {
			break
		}
		err = jsonEncoder.Encode(uploadStatusForProgress(p))
		if err != nil {
			break
		}
		_, err = io.WriteString(w, "\n\n")
		if err != nil {
			break
		}

		flusher.Flush()
	}

	if err != nil {
		log.WithError(err).Print("Error writing progress")
	}
}

func (ss *StorageService) setStatusExtraHandler(w http.ResponseWriter, r *http.Request) {
	// get id from request
	fileId, err := ss.getIdFromRequest(r)
	if err != nil {
		log.Println("error finding id for status extra", err.Error())
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var extra interface{}
	if err := json.NewDecoder(r.Body).Decode(&extra); err != nil {
		log.Println("error decoding status extra", err.Error())
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := ss.storageClient.setStatusExtra(fileId, extra); err != nil {
		if err == ErrNotFound {
			http.Error(w, `{"error": "not found"}`, http.StatusNotFound)
		} else if err == ErrRetrievingItem {
			http.Error(w, `{"error": "bad request"}`, http.StatusBadRequest)
		} else {
			log.Println("error setting status extra", err.Error())
			http.Error(w, fmt.Sprintf(`{"error": %#v}`, err.Error()), http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, `{"status": "ok"}`)
}

func (ss *StorageService) statusHandler(w http.ResponseWriter, r *http.Request) {
	// get id from request
	fileId, err := ss.getIdFromRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	counter, size, err := ss.storageClient.status(fileId)
	if err != nil {
		if err == ErrNotFound {
			http.Error(w, `{"error": "not found"}`, http.StatusNotFound)
		} else if err == ErrRetrievingItem {
			http.Error(w, `{"error": "bad request"}`, http.StatusBadRequest)
		} else {
			http.Error(w, fmt.Sprintf(`{"error": %#v}`, err.Error()), http.StatusInternalServerError)
		}
		return
	}

	status := uploadStatusForCounter(counter, size)
	err = json.NewEncoder(w).Encode(status)
	if err != nil {
		log.WithError(err).Errorf("Error writing status '%#v'", status)
	}
}

func (ss *StorageService) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func resolveContainerFromConfig(config map[string]string) (stow.Container, error) {
	provider := config[ConfigProvider]
	containerName := config[ConfigContainer]

	return ResolveContainer(provider, containerName, config)
}

func MakeStorageService(storageClient *StowClient, port int) *StorageService {
	return &StorageService{
		storageClient: storageClient,
		port:          port,
	}
}

func (ss *StorageService) Start(port int) error {
	r := mux.NewRouter()
	r.HandleFunc("/v1/archive", ss.uploadHandler).Queries("archiveID", "{archiveID}").Methods("POST")
	r.HandleFunc("/v1/archive/{archiveID}", ss.uploadHandler).Methods("POST")
	r.HandleFunc("/v1/archive", ss.downloadHandler).Methods("GET")
	r.HandleFunc("/v1/status", ss.statusHandler).Methods("GET")
	r.HandleFunc("/v1/status", ss.setStatusExtraHandler).Methods("POST")
	r.HandleFunc("/v1/events", ss.eventsHandler).Methods("GET")
	r.HandleFunc("/v1/archive", ss.deleteHandler).Methods("DELETE")
	r.HandleFunc("/healthz", ss.healthHandler).Methods("GET")

	address := fmt.Sprintf(":%v", port)

	r.Use(fission.LoggingMiddleware)
	err := http.ListenAndServe(address, &ochttp.Handler{
		Handler: r,
		// Propagation: &b3.HTTPFormat{},
	})

	return err
}

func RunStorageService(port int, enablePruner bool, readWriteConfig map[string]string, readOnlyConfigs []map[string]string) error {
	// setup a signal handler for SIGTERM
	fission.SetupStackTraceHandler()

	// initialize logger
	log.SetLevel(log.InfoLevel)

	// create a storage client
	readWriteContainer, err := resolveContainerFromConfig(readWriteConfig)
	if err != nil {
		return err
	}
	readOnlyContainers := []stow.Container{}
	for _, readOnlyConfig := range readOnlyConfigs {
		readOnlyContainer, err := resolveContainerFromConfig(readOnlyConfig)
		if err != nil {
			return err
		}
		readOnlyContainers = append(readOnlyContainers, readOnlyContainer)
	}

	storageClient := MakeStowClient(readWriteContainer, readOnlyContainers...)

	// create http handlers
	storageService := MakeStorageService(storageClient, port)

	// enablePruner prevents storagesvc unit test from needing to talk to kubernetes
	if enablePruner {
		// get the prune interval and start the archive pruner
		pruneInterval, err := strconv.Atoi(os.Getenv("PRUNE_INTERVAL"))
		if err != nil {
			pruneInterval = defaultPruneInterval
		}
		pruner, err := MakeArchivePruner(storageClient, time.Duration(pruneInterval))
		if err != nil {
			return errors.Wrap(err, "Error creating archivePruner")
		}
		go pruner.Start()
	}

	log.Info("Starting storage service...")
	storageService.Start(port)

	return nil
}
