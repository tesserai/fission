package fetcher

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/fission/fission/environments/fetcher/tarextract"

	"github.com/docker/distribution"
	"github.com/docker/distribution/reference"
	"github.com/tesserai/docker-registry-client/registry"
)

type (
	dockerCreds struct {
		username string
		password string
	}

	DockerBlobFetcher struct {
		defaultRegistryURL string
		transport          http.RoundTripper

		credsByDomain map[string]dockerCreds
	}
)

func MakeDockerBlobFetcher(defaultRegistryURL string, transport http.RoundTripper) *DockerBlobFetcher {
	if transport == nil {
		transport = http.DefaultTransport
	}
	return &DockerBlobFetcher{
		defaultRegistryURL: defaultRegistryURL,
		transport:          transport,
		credsByDomain:      map[string]dockerCreds{},
	}

}

func (df *DockerBlobFetcher) registryForDomain(domain string) *registry.Registry {
	var url string

	if domain != "" {
		url = "https://" + domain
	} else {
		url = df.defaultRegistryURL
	}

	creds := df.credsByDomain[domain]

	return &registry.Registry{
		URL: url,
		Client: &http.Client{
			Transport: &registry.BasicTransport{
				Transport: df.transport,
				URL:       url,
				Username:  creds.username,
				Password:  creds.password,
			},
		},
		Logf: registry.Log,
	}
}

func (df *DockerBlobFetcher) SetBasicAuthForDomain(domain, username, password string) {
	df.credsByDomain[domain] = dockerCreds{username, password}
}

func (df *DockerBlobFetcher) DownloadFinalLayer(ctx context.Context, imageReference string, tmpPath string) error {
	ref, err := reference.ParseAnyReference(imageReference)
	if err != nil {
		return err
	}
	named, ok := ref.(reference.Named)
	if !ok {
		return fmt.Errorf("Cannot parse image reference into something fetchable: %s", imageReference)
	}

	hub := df.registryForDomain(reference.Domain(named))

	imageName := reference.Path(named)
	var imageTag string
	if tagged, ok := ref.(reference.Tagged); ok {
		imageTag = tagged.Tag()
	}
	manifest, err := hub.Manifest(ctx, imageName, imageTag)
	if err != nil {
		return err
	}

	var lastLayer distribution.Descriptor
	for _, layer := range manifest.References() {
		lastLayer = layer
	}

	reader, err := hub.DownloadBlob(ctx, imageName, lastLayer.Digest)
	if err != nil {
		return err
	}
	defer reader.Close()

	verifier := lastLayer.Digest.Verifier()
	hashingReader := io.TeeReader(reader, verifier)
	err = tarextract.ExtractTarGz(hashingReader, tmpPath)
	if err != nil {
		return err
	}

	if !verifier.Verified() {
		return fmt.Errorf("Downloaded blob failed to match digest: %#v", lastLayer.Digest)
	}

	return nil
}
