package main

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/miracl/casper/lib/source"
)

var errSourceFormat = errors.New("Sources invalid format")

type sourceFormatError struct {
	msg string
	err error
}

func (e sourceFormatError) Error() string {
	s := fmt.Sprintf("Invalid source definition: %v", e.msg)
	if e.err != nil {
		s = fmt.Sprintf("%v (Err:%v)", s, e.err)
	}
	return s
}

const configScheme = "config"

func getMultiSourcer(srcs []string) (*source.Source, error) {
	sourceTypes := map[string]getSourcer{
		configScheme: getConfigSource,
		"file":       getFileSource,
	}

	sourceList := make([]source.ValuesSourcer, len(srcs))
	for i, s := range srcs {
		u, err := url.Parse(s)
		if err != nil {
			return nil, err
		}

		if u.Scheme == "" {
			// Default to config
			u = &url.URL{
				Scheme:   configScheme,
				RawQuery: s,
			}
		}

		getSourcer, ok := sourceTypes[u.Scheme]
		fmt.Println(getSourcer, ok)
		if !ok {
			return nil, errSourceFormat
		}

		sourceList[i], err = getSourcer(u)
		if err != nil {
			return nil, err
		}
	}

	return source.NewMultiSourcer(sourceList...)
}

type getSourcer func(u *url.URL) (*source.Source, error)

func getConfigSource(u *url.URL) (*source.Source, error) {
	body := map[string]interface{}{}
	for k, v := range u.Query() {
		if len(v) > 1 {
			body[k] = v
		}

		body[k] = v[0]
	}

	return source.NewSource(body), nil
}

func getFileSource(u *url.URL) (*source.Source, error) {
	path := u.Hostname()
	pathSlice := strings.Split(path, ".")
	format := pathSlice[len(pathSlice)-1]

	r, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	fmt.Println(r, format)
	s, err := source.NewFileSource(r, format)
	if err != nil {
		return nil, sourceFormatError{"unable to create file source", err}
	}

	return s, nil
}
