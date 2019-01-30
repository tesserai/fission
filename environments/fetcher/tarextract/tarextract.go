package tarextract

// hat tip https://gist.github.com/indraniel/1a91458984179ab4cf80

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
)

func ExtractTarGz(gzipStream io.Reader, destination string) error {
	uncompressedStream, err := gzip.NewReader(gzipStream)
	if err != nil {
		return errors.Wrap(err, "new gzip reader")
	}

	tarReader := tar.NewReader(uncompressedStream)

	createdDirs := map[string]bool{}

	for {
		header, err := tarReader.Next()

		if err == io.EOF {
			break
		}

		if err != nil {
			return errors.Wrap(err, "reading next entry")
		}

		name := filepath.Clean(header.Name)
		if strings.HasPrefix(name, "..") {
			return fmt.Errorf("invalid header name: %s", name)
		}
		path := filepath.Join(destination, name)

		switch header.Typeflag {
		case tar.TypeDir:
			if !createdDirs[path] {
				if err := os.MkdirAll(path, 0755); err != nil {
					return errors.Wrap(err, "mkdir failed")
				}
				createdDirs[path] = true
			}
		case tar.TypeReg:
			parent := filepath.Dir(path)
			if !createdDirs[parent] {
				if err := os.MkdirAll(parent, 0755); err != nil {
					return errors.Wrap(err, "mkdir failed")
				}
				createdDirs[parent] = true
			}
			err := func() error {
				outFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, os.FileMode(header.Mode&0777))
				if err != nil {
					return errors.Wrap(err, "create failed")
				}
				defer outFile.Close()
				if _, err := io.Copy(outFile, tarReader); err != nil {
					return errors.Wrap(err, "copy failed")
				}

				return nil
			}()
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("unknown header type: %s in %s",
				header.Typeflag,
				header.Name)
		}
	}

	return nil
}
