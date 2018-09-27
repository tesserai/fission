// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package multipartformdata

import (
	"io"
	"mime/multipart"
	"net/textproto"
)

type FormFileVisitor func(filename string, header textproto.MIMEHeader, reader io.Reader) (func() error, error)

// TODO(adg,bradfitz): find a way to unify the DoS-prevention strategy here
// with that of the http package's ParseForm.

// ReadForm parses an entire multipart message whose parts have
// a Content-Disposition of "form-data".
func ReadForm(r *multipart.Reader, visitor FormFileVisitor) error {
	var err error

	removers := []func() error{}
	defer func() {
		if err != nil {
			for _, remover := range removers {
				remover()
			}
		}
	}()

	for {
		p, err := r.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		name := p.FormName()
		if name == "" {
			continue
		}
		filename := p.FileName()

		remover, err := visitor(filename, p.Header, p)
		if err != nil {
			return err
		}
		removers = append(removers, remover)
	}

	return nil
}
